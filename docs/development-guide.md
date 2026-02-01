# Desktop 开发指南

## 工作目录说明

### PowerShell 脚本（推荐）

PowerShell 脚本（`.ps1`）设计为**从项目根目录运行**：

```
C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server\
```

优势：
- 统一的工作目录，便于管理
- 自动请求管理员权限（VPN 功能需要）
- 更好的错误处理和输出
- 支持参数传递

### Batch 脚本（传统）

Batch 脚本（`.bat`）需要**从 desktop 子目录运行**：

```
C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server\desktop\
```

## 开发模式

### 使用 PowerShell（推荐）

```powershell
# 在项目根目录运行
cd C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server

# 启动开发服务器（自动请求管理员权限）
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1

# 指定版本
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1 -BuildVersion "v0.3.0"

# 指定端口（默认 34115）
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1 -Port 35000
```

### 使用 Batch

```bash
# 切换到 desktop 目录
cd desktop

# 启动开发服务器
scripts\dev.bat
```

## 构建

### 使用 PowerShell（推荐）

```powershell
# 在项目根目录运行
cd C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server

# 构建（默认 amd64）
powershell -ExecutionPolicy Bypass -File desktop\scripts\build.ps1

# 指定版本和架构
powershell -ExecutionPolicy Bypass -File desktop\scripts\build.ps1 -BuildVersion "v0.3.0" -GoArch "amd64"

# 指定服务器地址
powershell -ExecutionPolicy Bypass -File desktop\scripts\build.ps1 -BuildAddress "https://api.example.com"
```

### 使用 Batch

```bash
# 切换到 desktop 目录
cd desktop

# 构建
scripts\build.bat

# 指定版本
set BUILD_VERSION=v0.3.0
scripts\build.bat
```

## 输出位置

构建输出统一在：

```
desktop\build\bin\
├── awecloud-signaling-desktop.exe              # 默认输出
└── awecloud-signaling-v0.2.0-windows-amd64.exe # 带版本号的副本
```

## 管理员权限说明

### 为什么需要管理员权限？

Desktop 应用使用 Tailscale VPN 功能，需要：
- 创建虚拟网络适配器（Wintun）
- 修改系统路由表
- 管理网络连接

### PowerShell 自动提权

`dev.ps1` 脚本会自动检测权限并请求提升：

```powershell
# 脚本会自动检测并弹出 UAC 提示
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1
```

### Batch 手动提权

`dev.bat` 脚本也会自动请求管理员权限，但体验不如 PowerShell。

### 手动以管理员身份运行

右键点击 PowerShell 图标 → "以管理员身份运行"，然后执行脚本。

## Wintun 驱动

### 自动下载

开发脚本会自动下载 Wintun 驱动（v0.14.1）到：

```
desktop\build\bin\wintun.dll
```

### 手动下载

如果自动下载失败，可以手动下载：

1. 访问：https://www.wintun.net/builds/wintun-0.14.1.zip
2. 解压后复制 `bin\amd64\wintun.dll` 到 `desktop\build\bin\`

## 常见问题

### 1. 执行策略错误

```
无法加载文件 dev.ps1，因为在此系统上禁止运行脚本
```

解决方案：使用 `-ExecutionPolicy Bypass` 参数

```powershell
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1
```

### 2. 权限不足

```
[WARN] Not running as Administrator!
```

解决方案：
- PowerShell 脚本会自动请求提权
- 或手动以管理员身份运行 PowerShell

### 3. Wintun 下载失败

```
[ERROR] Failed to download wintun
```

解决方案：
- 检查网络连接
- 手动下载并放置到 `desktop\build\bin\wintun.dll`
- VPN 功能将不可用，但应用仍可运行

### 4. 端口被占用

```
Error: listen tcp :34115: bind: Only one usage of each socket address
```

解决方案：指定其他端口

```powershell
powershell -ExecutionPolicy Bypass -File desktop\scripts\dev.ps1 -Port 35000
```

## 调试技巧

### 1. 查看详细日志

开发模式会显示详细的构建和运行日志。

### 2. 前端热重载

修改 `desktop\frontend\src\` 下的文件会自动触发热重载。

### 3. 后端重启

修改 Go 代码后，Wails 会自动重新编译并重启应用。

### 4. 清理缓存

```powershell
# 清理前端缓存
Remove-Item desktop\frontend\node_modules -Recurse -Force
Remove-Item desktop\frontend\dist -Recurse -Force

# 清理构建缓存
Remove-Item desktop\build -Recurse -Force
Remove-Item desktop\.tmp -Recurse -Force

# 重新安装依赖
cd desktop\frontend
npm install
```

## 目录结构

```
awecloud-signaling-server\          # 项目根目录（PowerShell 脚本工作目录）
├── desktop\                        # Desktop 子项目（Batch 脚本工作目录）
│   ├── scripts\
│   │   ├── dev.ps1                # PowerShell 开发脚本（推荐）
│   │   ├── build.ps1              # PowerShell 构建脚本（推荐）
│   │   ├── dev.bat                # Batch 开发脚本
│   │   └── build.bat              # Batch 构建脚本
│   ├── frontend\                  # Vue 3 前端
│   ├── internal\                  # Go 后端
│   ├── build\
│   │   └── bin\                   # 构建输出
│   │       ├── wintun.dll         # Wintun 驱动（自动下载）
│   │       └── *.exe              # 可执行文件
│   └── .tmp\                      # 临时文件（gitignored）
└── ...
```

## 参考资料

- [Wails v3 文档](https://v3alpha.wails.io/)
- [Wintun 驱动](https://www.wintun.net/)
- [Tailscale 文档](https://tailscale.com/kb/)
