# Build Directory

构建目录用于存放应用程序的所有构建文件和资源。

## 目录结构

```
build/
├── bin/          # 输出目录
├── darwin/       # macOS 特定文件
├── linux/        # Linux 特定文件
├── windows/      # Windows 特定文件
├── tray/         # 系统托盘图标
├── appicon.png   # 应用图标
└── config.yml    # Wails 配置
```

## 构建依赖安装

### Ubuntu / Debian

构建 Linux 原生应用需要安装 GTK3 和 WebKit2GTK：

```bash
# 更新包列表
sudo apt update

# 安装 Linux 构建依赖
sudo apt install -y \
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
    libsoup-3.0-dev \
    build-essential \
    pkg-config
```

### 交叉编译 Windows 程序（在 Ubuntu 上）

在 Linux 上交叉编译 Windows 程序需要安装 MinGW-w64：

```bash
# 安装 MinGW-w64 交叉编译工具链
sudo apt install -y \
    gcc-mingw-w64-x86-64 \
    g++-mingw-w64-x86-64

# 设置默认的 MinGW 版本（选择 posix 线程模型）
sudo update-alternatives --set x86_64-w64-mingw32-gcc /usr/bin/x86_64-w64-mingw32-gcc-posix
sudo update-alternatives --set x86_64-w64-mingw32-g++ /usr/bin/x86_64-w64-mingw32-g++-posix
```

交叉编译 Windows 时设置环境变量：

```bash
# 交叉编译 Windows amd64
CGO_ENABLED=1 \
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
GOOS=windows \
GOARCH=amd64 \
go build -o build/bin/app.exe
```

### macOS

macOS 需要安装 Xcode Command Line Tools：

```bash
xcode-select --install
```

### 安装 Wails CLI

所有平台都需要安装 Wails CLI：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

## 构建命令

```bash
# 开发模式
cd desktop
./scripts/dev.sh

# 构建当前平台
./scripts/build.sh

# 构建指定平台
./scripts/build.sh linux
./scripts/build.sh windows
./scripts/build.sh darwin
```

## 平台特定文件

### macOS (darwin/)

- `Info.plist` - 生产构建使用的 plist 文件
- `Info.dev.plist` - 开发构建使用的 plist 文件

### Windows (windows/)

- `icon.ico` - 应用图标
- `info.json` - 应用详情（用于安装程序和属性）
- `wails.exe.manifest` - 应用清单文件
- `installer/` - Windows 安装程序文件

### Linux (linux/)

- `appicon.png` - 应用图标
- `*.desktop` - 桌面入口文件

## 常见问题

### GTK3 not found

```
Package gtk+-3.0 was not found in the pkg-config search path.
```

解决方案：安装 GTK3 开发库

```bash
sudo apt install libgtk-3-dev
```

### webkit2gtk-4.1 not found

```
Package 'webkit2gtk-4.1', required by 'virtual:world', not found
```

解决方案：安装 WebKit2GTK 开发库

```bash
sudo apt install libwebkit2gtk-4.1-dev
```

### libsoup-3.0 not found

```
Package 'libsoup-3.0', required by 'virtual:world', not found
```

解决方案：安装 libsoup3 开发库

```bash
sudo apt install libsoup-3.0-dev
```

### 一键安装所有依赖

```bash
sudo apt install -y \
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
    libsoup-3.0-dev \
    build-essential \
    pkg-config \
    gcc-mingw-w64-x86-64 \
    g++-mingw-w64-x86-64
```
