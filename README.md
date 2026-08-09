# AWECloud Signaling Desktop

Desktop 客户端应用 - 基于 Wails 的跨平台桌面应用

## 项目信息

- **技术栈**: Wails v3 + Go + Vue 3 + TypeScript
- **主项目**: https://github.com/open-beagle/awecloud-signaling-server
- **设计文档**: 见主项目 `docs/design_desktop.md`

## 快速启动

### 前置要求

- Go 1.25+
- Node.js 18+
- 与 `go.mod` 一致的 Wails CLI

安装 Wails CLI:

```bash
WAILS_VERSION="$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)"
go install "github.com/wailsapp/wails/v3/cmd/wails3@${WAILS_VERSION}"
```

正式构建直接使用仓库中的 `frontend/bindings`。修改 Go 服务接口后，需要重新生成并提交绑定：

```bash
wails3 generate bindings -clean=true ./...
```

Windows PowerShell：

```powershell
$WailsVersion = go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3
go install "github.com/wailsapp/wails/v3/cmd/wails3@$WailsVersion"
```

### 开发模式

#### Linux

```bash
# 安装前端依赖
cd frontend && npm install && cd ..

# 启动开发模式（热重载）
wails3 dev

# 或使用脚本
./scripts/dev.sh
```

#### Windows

**方式一：管理员终端直接运行（推荐）：**

右键 Windows Terminal → "以管理员身份运行"，然后：

```powershell
# 从项目根目录 awecloud-signaling-server\ 运行

# 开发模式
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1

# 构建代码
powershell -ExecutionPolicy Bypass -File desktop\scripts\build.ps1 -GoArch amd64
```

**方式二：普通终端提权运行：**

```powershell
# 从项目根目录 awecloud-signaling-server\ 运行

# 开发模式（需要管理员权限）
Start-Process powershell -Verb RunAs -ArgumentList "-NoExit -Command cd '$((Get-Location).Path)'; powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1"

# 构建代码（需要管理员权限）
Start-Process powershell -Verb RunAs -ArgumentList "-NoExit -Command cd '$((Get-Location).Path)'; powershell -ExecutionPolicy Bypass -File desktop\scripts\build.ps1 -GoArch amd64"
```

**说明：**

- 方式二通过 `-Command cd` 显式切换目录，避免 Win11 上 `-WorkingDirectory` 被忽略的问题
- VPN 功能需要管理员权限才能正常工作

**或使用传统 Batch 脚本（从 desktop\ 目录运行）：**

```bash
# 从 desktop\ 目录运行
cd desktop

# 构建代码
scripts\build.bat

# 调试代码
scripts\dev.bat
```

### 构建

打包前必须从模板创建本地配置：

```bash
cp .env.example .env
```

`SIGNALING_ADDRESS` 必须非空。`.env.example` 只作为字段模板，正式打包不会回退读取它。

```bash
# Linux/macOS，支持多平台
./scripts/build.sh

# 指定平台构建
PLATFORMS=windows/amd64 ./scripts/build.sh
PLATFORMS=linux/amd64,windows/amd64 ./scripts/build.sh
PLATFORMS=darwin/amd64,darwin/arm64 ./scripts/build.sh  # 需 macOS 环境
```

Windows 本地打包使用 `scripts\build.ps1` 或 `scripts\build.bat`。GitHub Actions 使用组织级 `SIGNALING_ADDRESS` Secret 生成 `.env` 后调用同一构建脚本。

支持的平台：

- `windows/amd64` - Windows 64 位（wintun.dll 自动嵌入）
- `linux/amd64` - Linux 64 位
- `darwin/amd64` - macOS Intel
- `darwin/arm64` - macOS Apple Silicon

桌面开发产物使用固定名称和固定位置；版本号、提交号与构建时间写入二进制元数据，不进入文件名：

- Windows：`build/bin/awecloud-signaling-desktop.exe`
- Linux：`build/bin/awecloud-signaling-desktop`
- macOS：`build/bin/awecloud-signaling-desktop.zip`

重复构建会覆盖同一路径。Windows 开发与验收不得改用随机临时可执行路径，避免产生新的防火墙应用身份。

## 文档规范

### 3.1 根目录文档规范

根目录下不允许 README.md 之外的文档。

### 3.2 文档基地

所有文档统一存放在 `docs/` 目录下。

### 3.3 文档命名规范

docs 下所有文档使用小写命名。

### 3.4 文档分类规范

- **设计类**: `design-` 开头（如 `design-logo.md`）
- **开发进度类**: `progress-` 开头（如 `progress-mvp.md`）
- **计划类**: `plan-` 开头（如 `plan-roadmap.md`）
- **开发进展**: `development-` 开头（如 `development-guide.md`）

## 文档结构

```
docs/
├── development-guide.md    # 开发指南
├── user-guide.md          # 用户手册
├── design-logo.md         # Logo 设计说明
├── progress-mvp.md        # MVP 开发进度
├── progress-testing.md    # 测试指南
├── progress-results.md    # 测试结果
└── plan-release.md        # 发布计划
```

详见 [docs/README.md](docs/README.md)

## 许可证

与主项目相同
