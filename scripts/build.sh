#!/bin/bash

# Desktop 构建脚本 (Wails v3)

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

# 检查 wails3 是否安装（仅在需要生成绑定时必需）
WAILS3_AVAILABLE=false
if command -v wails3 &> /dev/null; then
    WAILS3_AVAILABLE=true
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
    
    if ! pkg-config --exists gtk+-3.0 2>/dev/null; then
        echo "GTK3 not found, skipping installation (run manually if needed)"
        return
    fi
    
    if ! pkg-config --exists webkit2gtk-4.1 2>/dev/null && ! pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
        echo "WebKit2GTK not found, skipping installation (run manually if needed)"
        return
    fi
    
    echo -e "${GREEN}✓ Linux build dependencies OK${NC}"
}

# installWindowsDeps 安装Windows交叉编译依赖
installWindowsDeps() {
    echo -e "${YELLOW}Checking Windows cross-compilation dependencies...${NC}"
    
    # Windows 交叉编译不需要 CGO，所以不需要 MinGW
    echo -e "${GREEN}✓ Windows cross-compilation dependencies OK${NC}"
}

# generateWindowsResources 生成 Windows 资源文件（图标嵌入）
generateWindowsResources() {
    local ARCH="$1"
    echo -e "${YELLOW}Generating Windows resources (icon embedding)...${NC}"
    
    # 检查 go-winres 是否安装
    if ! command -v go-winres &> /dev/null; then
        echo -e "${YELLOW}Installing go-winres...${NC}"
        go install github.com/tc-hib/go-winres@latest
    fi
    
    # 创建 winres 目录
    mkdir -p winres
    
    # 复制图标文件
    if [ -f "build/windows/icon.ico" ]; then
        cp "build/windows/icon.ico" "winres/icon.ico"
    elif [ -f "build/appicon.png" ]; then
        cp "build/appicon.png" "winres/icon.png"
    fi
    
    # 生成正确的 manifest 文件（替换模板变量）
    cat > "winres/app.manifest" << 'EOF'
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
    <assemblyIdentity type="win32" name="com.awecloud.signaling.desktop" version="1.0.0.0" processorArchitecture="*"/>
    <dependency>
        <dependentAssembly>
            <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
        </dependentAssembly>
    </dependency>
    <asmv3:application>
        <asmv3:windowsSettings>
            <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
            <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">permonitorv2,permonitor</dpiAwareness>
        </asmv3:windowsSettings>
    </asmv3:application>
</assembly>
EOF
    
    # 创建 winres.json 配置文件
    cat > "winres/winres.json" << EOF
{
    "RT_GROUP_ICON": {
        "APP": {
            "0000": "icon.ico"
        }
    },
    "RT_MANIFEST": {
        "#1": {
            "0000": "app.manifest"
        }
    },
    "RT_VERSION": {
        "#1": {
            "0000": {
                "fixed": {
                    "file_version": "${BUILD_VERSION}.${BUILD_NUMBER}",
                    "product_version": "${BUILD_VERSION}.${BUILD_NUMBER}"
                },
                "info": {
                    "0409": {
                        "CompanyName": "AWECloud",
                        "FileDescription": "AWECloud Signaling Desktop",
                        "FileVersion": "${BUILD_VERSION}",
                        "InternalName": "awecloud-signaling-desktop",
                        "LegalCopyright": "Copyright © 2025 AWECloud. All rights reserved.",
                        "OriginalFilename": "awecloud-signaling-desktop.exe",
                        "ProductName": "Signaling Desktop",
                        "ProductVersion": "${BUILD_VERSION}"
                    }
                }
            }
        }
    }
}
EOF
    
    # 生成 .syso 文件
    echo "Running: go-winres make --arch ${ARCH}"
    go-winres make --arch "${ARCH}"
    
    if [ -f "rsrc_windows_${ARCH}.syso" ]; then
        echo -e "${GREEN}✓ Windows resources generated: rsrc_windows_${ARCH}.syso${NC}"
    else
        echo -e "${RED}✗ Failed to generate Windows resources${NC}"
        return 1
    fi
}

# cleanWindowsResources 清理 Windows 资源文件
cleanWindowsResources() {
    rm -f rsrc_windows_*.syso
    rm -rf winres
}

# checkMacOSDeps 检查macOS构建环境
checkMacOSDeps() {
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "${GREEN}✓ Building on macOS${NC}"
        export CGO_ENABLED=1
    else
        echo -e "${YELLOW}╔════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${YELLOW}║  macOS 交叉编译不支持                                      ║${NC}"
        echo -e "${YELLOW}║  Wails 需要 CGO 调用 macOS 原生框架（Cocoa/WebKit）        ║${NC}"
        echo -e "${YELLOW}║  请在 macOS 机器上构建，或使用 GitHub Actions              ║${NC}"
        echo -e "${YELLOW}╚════════════════════════════════════════════════════════════╝${NC}"
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

# 构建前端
echo -e "${YELLOW}Building frontend...${NC}"
cd frontend
npm run build
cd ..

# 生成绑定（如果 wails3 可用且绑定目录不存在）
if [ "$WAILS3_AVAILABLE" = true ]; then
    echo -e "${YELLOW}Generating bindings...${NC}"
    wails3 generate bindings
elif [ -d "frontend/bindings" ]; then
    echo -e "${YELLOW}wails3 not available, using existing bindings...${NC}"
else
    echo -e "${RED}Error: wails3 not available and no existing bindings found${NC}"
    echo "Please install wails3 or ensure frontend/bindings directory exists"
    exit 1
fi

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
            generateWindowsResources "${ARCH}"
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
        BUILD_OUTPUT="${OUTPUT_DIR}/awecloud-signaling-desktop.exe"
    else
        BUILD_OUTPUT="${OUTPUT_DIR}/awecloud-signaling-desktop"
    fi
    
    # 构建 ldflags
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
    
    # 设置环境变量
    export GOOS="${OS}"
    export GOARCH="${ARCH}"
    
    # Windows 不需要 CGO
    if [ "$OS" = "windows" ]; then
        export CGO_ENABLED=0
    else
        export CGO_ENABLED=1
    fi
    
    # 执行构建
    echo "Building with: go build -tags production -trimpath -ldflags \"${LDFLAGS}\" -o ${BUILD_OUTPUT}"
    go build -tags production -trimpath -ldflags "${LDFLAGS}" -o "${BUILD_OUTPUT}"
    
    # 检查构建结果
    if [ -f "${BUILD_OUTPUT}" ]; then
        echo -e "${GREEN}✓ Build successful: ${BUILD_OUTPUT}${NC}"
        
        # Windows: 清理资源文件
        if [ "$OS" = "windows" ]; then
            cleanWindowsResources
        fi
        
        # macOS: 打包为 .app.zip
        if [ "$OS" = "darwin" ]; then
            echo -e "${YELLOW}Packaging macOS .app bundle...${NC}"
            
            APP_BUNDLE_NAME="Signaling Desktop.app"
            ZIP_NAME="awecloud-signaling-${BUILD_VERSION}-${OS}-${ARCH}.zip"
            
            # 创建 .app 目录结构
            rm -rf "${OUTPUT_DIR}/${APP_BUNDLE_NAME}"
            mkdir -p "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/MacOS"
            mkdir -p "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/Resources"
            
            # 复制可执行文件
            cp "${BUILD_OUTPUT}" "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/MacOS/awecloud-signaling-desktop"
            
            # 创建 Info.plist
            cat > "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleName</key>
    <string>Signaling Desktop</string>
    <key>CFBundleExecutable</key>
    <string>awecloud-signaling-desktop</string>
    <key>CFBundleIdentifier</key>
    <string>com.awecloud.signaling.desktop</string>
    <key>CFBundleVersion</key>
    <string>${BUILD_VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${BUILD_VERSION}</string>
    <key>CFBundleIconFile</key>
    <string>iconfile</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.13.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSHumanReadableCopyright</key>
    <string>Copyright © 2025 AWECloud. All rights reserved.</string>
</dict>
</plist>
EOF
            
            # 复制图标（如果存在）
            if [ -f "build/darwin/icons.icns" ]; then
                cp "build/darwin/icons.icns" "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/Resources/iconfile.icns"
            elif [ -f "build/appicon.png" ]; then
                cp "build/appicon.png" "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/Resources/iconfile.png"
            fi
            
            # 创建 PkgInfo 文件
            echo -n "APPL????" > "${OUTPUT_DIR}/${APP_BUNDLE_NAME}/Contents/PkgInfo"
            
            # 创建 zip 压缩包
            # 临时签名（Ad-hoc signing）以修复 Apple Silicon 上的应用损坏问题
            echo -e "${YELLOW}Applying ad-hoc signature...${NC}"
            codesign --force --deep --sign - "${OUTPUT_DIR}/${APP_BUNDLE_NAME}"

            cd "${OUTPUT_DIR}"
            zip -r "${ZIP_NAME}" "${APP_BUNDLE_NAME}"
            cd - > /dev/null
            
            # 清理 .app 目录和原始二进制文件
            rm -rf "${OUTPUT_DIR}/${APP_BUNDLE_NAME}"
            rm -f "${BUILD_OUTPUT}"
            
            echo -e "${GREEN}✓ Output: ${OUTPUT_DIR}/${ZIP_NAME}${NC}"
            FILE_SIZE=$(ls -lh "${OUTPUT_DIR}/${ZIP_NAME}" | awk '{print $5}')
            echo "  File size: ${FILE_SIZE}"
        else
            # 非 macOS: 复制到输出目录并重命名
            if [ "${BUILD_OUTPUT}" != "${OUTPUT_DIR}/${OUTPUT_NAME}" ]; then
                cp "${BUILD_OUTPUT}" "${OUTPUT_DIR}/${OUTPUT_NAME}"
            fi
            echo -e "${GREEN}✓ Output: ${OUTPUT_DIR}/${OUTPUT_NAME}${NC}"
            # 显示文件大小
            FILE_SIZE=$(ls -lh "${OUTPUT_DIR}/${OUTPUT_NAME}" | awk '{print $5}')
            echo "  File size: ${FILE_SIZE}"
        fi
    else
        echo -e "${RED}✗ Build failed for ${OS}/${ARCH}${NC}"
        echo -e "${RED}Expected output: ${BUILD_OUTPUT}${NC}"
        ls -la "${OUTPUT_DIR}/" 2>/dev/null || echo "Output directory not found"
        exit 1
    fi
done

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}All builds completed successfully!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Output directory: ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}/"
