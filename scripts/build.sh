#!/bin/bash

# Desktop 构建脚本

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 版本信息
BUILD_VERSION="${BUILD_VERSION:-dev}"
BUILD_ADDRESS="${BUILD_ADDRESS:-}"  # 默认 Server 地址（可选）
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_NUMBER=$(git rev-list --count HEAD 2>/dev/null || echo "0")
BUILD_DATE=$(date -u '+%Y-%m-%d_%H:%M:%S')

# 目标平台
PLATFORMS="${PLATFORMS:-windows/amd64}"  # 默认仅构建 Windows amd64
# 可选值：linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64

OUTPUT_DIR="./build/bin"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}AWECloud Signaling Desktop Builder${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Version:      ${BUILD_VERSION}"
echo "Build Number: ${BUILD_NUMBER}"
echo "Address:      ${BUILD_ADDRESS:-<not set>}"
echo "Git Commit:   ${GIT_COMMIT}"
echo "Build Date:   ${BUILD_DATE}"
echo "Platforms:    ${PLATFORMS}"
echo ""

# 检查 wails 是否安装
if ! command -v wails &> /dev/null; then
    echo -e "${RED}Error: wails command not found${NC}"
    echo ""
    echo "Please install Wails first:"
    echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    echo ""
    echo "Or visit: https://wails.io/docs/gettingstarted/installation"
    exit 1
fi

# 检查 Node.js 是否安装
if ! command -v node &> /dev/null; then
    echo -e "${RED}Error: node command not found${NC}"
    echo "Please install Node.js first: https://nodejs.org/"
    exit 1
fi

# installLinuxDeps 安装Linux构建依赖
installLinuxDeps() {
    echo -e "${YELLOW}Checking Linux build dependencies...${NC}"
    
    if ! pkg-config --exists gtk+-3.0; then
        echo "GTK3 not found, skipping installation (run manually if needed)"
        return
    fi
    
    if ! pkg-config --exists webkit2gtk-4.1 && ! pkg-config --exists webkit2gtk-4.0; then
        echo "WebKit2GTK not found, skipping installation (run manually if needed)"
        return
    fi
    
    # 创建webkit2gtk-4.0.pc软链接（如果需要）
    if pkg-config --exists webkit2gtk-4.1 && ! pkg-config --exists webkit2gtk-4.0; then
        echo -e "${YELLOW}Creating webkit2gtk-4.0.pc symlink...${NC}"
        for dir in /usr/lib/x86_64-linux-gnu/pkgconfig /usr/lib/pkgconfig /usr/local/lib/pkgconfig; do
            if [ -f "$dir/webkit2gtk-4.1.pc" ]; then
                sudo ln -sf "$dir/webkit2gtk-4.1.pc" "$dir/webkit2gtk-4.0.pc" 2>/dev/null || true
                break
            fi
        done
    fi
    
    echo -e "${GREEN}✓ Linux build dependencies OK${NC}"
}

# installWindowsDeps 安装Windows交叉编译依赖
installWindowsDeps() {
    echo -e "${YELLOW}Checking Windows cross-compilation dependencies...${NC}"
    
    if ! command -v x86_64-w64-mingw32-gcc &> /dev/null; then
        echo "MinGW-w64 not found, skipping installation (run manually if needed)"
        return
    fi
    
    echo -e "${GREEN}✓ Windows cross-compilation dependencies OK${NC}"
}

# checkMacOSDeps 检查macOS构建环境
checkMacOSDeps() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${GREEN}✓ Building on macOS${NC}"
    else
        echo -e "${YELLOW}Note: macOS cross-compilation from Linux is not supported${NC}"
        echo -e "${YELLOW}Skipping macOS build${NC}"
        return 1
    fi
}

# 安装前端依赖
echo -e "${YELLOW}Installing frontend dependencies...${NC}"
cd frontend
if [ ! -d "node_modules" ]; then
    npm install
else
    echo "Frontend dependencies already installed, skipping..."
fi
cd ..

# 创建输出目录
mkdir -p "${OUTPUT_DIR}"

# 解析平台列表
IFS=',' read -ra PLATFORM_ARRAY <<< "$PLATFORMS"

# 构建每个平台
for PLATFORM in "${PLATFORM_ARRAY[@]}"; do
    # 解析平台和架构
    IFS='/' read -r OS ARCH <<< "$PLATFORM"
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}Building for ${OS}/${ARCH}${NC}"
    echo -e "${GREEN}========================================${NC}"
    
    # 根据平台检查依赖
    case "$OS" in
        linux)
            installLinuxDeps
            ;;
        windows)
            installWindowsDeps
            ;;
        darwin)
            if ! checkMacOSDeps; then
                echo -e "${YELLOW}Skipping macOS build${NC}"
                continue
            fi
            ;;
    esac
    
    # 设置输出文件名
    OUTPUT_NAME="awecloud-signaling-${BUILD_VERSION}-${OS}-${ARCH}"
    if [ "$OS" = "windows" ]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    # 构建参数
    # macOS universal 需要特殊处理
    if [ "$OS" = "darwin" ] && [ "$ARCH" = "universal" ]; then
        BUILD_FLAGS="-clean -platform darwin/universal"
    else
        BUILD_FLAGS="-clean -platform ${OS}/${ARCH}"
    fi
    
    # 添加 ldflags
    LDFLAGS="-w -s"
    # Windows: 添加 -H windowsgui 隐藏控制台窗口
    if [ "$OS" = "windows" ]; then
        LDFLAGS="${LDFLAGS} -H windowsgui"
    fi
    # 注入版本信息到 version 包
    LDFLAGS="${LDFLAGS} -X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.Version=${BUILD_VERSION}'"
    LDFLAGS="${LDFLAGS} -X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.GitCommit=${GIT_COMMIT}'"
    LDFLAGS="${LDFLAGS} -X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildTime=${BUILD_DATE}'"
    LDFLAGS="${LDFLAGS} -X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildNumber=${BUILD_NUMBER}'"
    if [ -n "${BUILD_ADDRESS}" ]; then
        LDFLAGS="${LDFLAGS} -X 'github.com/open-beagle/awecloud-signaling-desktop/internal/config.buildAddress=${BUILD_ADDRESS}'"
    fi
    BUILD_FLAGS="${BUILD_FLAGS} -ldflags \"${LDFLAGS}\""
    
    # 执行构建
    echo "Building with: wails build ${BUILD_FLAGS}"
    eval "wails build ${BUILD_FLAGS}"
    
    # 检查构建结果
    if [ "$OS" = "darwin" ]; then
        # macOS 构建产物是 .app 包
        # Wails 可能生成不同的 .app 名称，自动检测
        APP_FILE=$(find build/bin -maxdepth 1 -name "*.app" -type d | head -n 1)
        
        if [ -n "$APP_FILE" ] && [ -d "$APP_FILE" ]; then
            APP_NAME=$(basename "$APP_FILE")
            echo -e "${GREEN}✓ Build successful: ${APP_FILE}${NC}"
            
            # 显示 .app 包大小
            APP_SIZE=$(du -sh "$APP_FILE" | awk '{print $1}')
            echo "  App size: ${APP_SIZE}"
            
            # 创建 zip 包（标准分发方式）
            ZIP_NAME="awecloud-signaling-${BUILD_VERSION}-${OS}-${ARCH}.zip"
            echo "Creating zip archive: ${ZIP_NAME}"
            cd build/bin
            zip -r -q "${ZIP_NAME}" "$APP_NAME"
            cd ../..
            
            # 显示 zip 包大小
            ZIP_SIZE=$(ls -lh "build/bin/${ZIP_NAME}" | awk '{print $5}')
            echo -e "${GREEN}✓ Created: build/bin/${ZIP_NAME} (${ZIP_SIZE})${NC}"
        else
            echo -e "${RED}✗ Build failed for ${OS}/${ARCH}${NC}"
            echo -e "${RED}No .app file found in build/bin/${NC}"
            echo "Contents of build/bin:"
            ls -la build/bin/ || echo "build/bin directory not found"
            exit 1
        fi
    else
        # Linux/Windows 构建产物是可执行文件
        if [ "$OS" = "windows" ]; then
            BUILD_OUTPUT="build/bin/awecloud-signaling-desktop.exe"
        else
            BUILD_OUTPUT="build/bin/awecloud-signaling-desktop"
        fi
        
        if [ -f "${BUILD_OUTPUT}" ]; then
            echo -e "${GREEN}✓ Build successful: ${BUILD_OUTPUT}${NC}"
            # 复制到输出目录并重命名
            cp "${BUILD_OUTPUT}" "${OUTPUT_DIR}/${OUTPUT_NAME}"
            echo -e "${GREEN}✓ Copied to: ${OUTPUT_DIR}/${OUTPUT_NAME}${NC}"
            # 显示文件大小
            FILE_SIZE=$(ls -lh "${OUTPUT_DIR}/${OUTPUT_NAME}" | awk '{print $5}')
            echo "  File size: ${FILE_SIZE}"
        else
            echo -e "${RED}✗ Build failed for ${OS}/${ARCH}${NC}"
            echo -e "${RED}Expected output: ${BUILD_OUTPUT}${NC}"
            ls -la build/bin/ || echo "build/bin directory not found"
            exit 1
        fi
    fi
done

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}All builds completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Output directory: ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}/"
