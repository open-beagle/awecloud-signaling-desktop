# Desktop 开发脚本 - Windows PowerShell 版本（简化版）
# 工作目录: awecloud-signaling-server\desktop\

param(
    [string]$BuildVersion = $env:BUILD_VERSION
)

# 设置工作目录为 desktop 目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopDir = Split-Path -Parent $ScriptDir
Set-Location $DesktopDir

# 读取版本号
if ([string]::IsNullOrEmpty($BuildVersion)) {
    if (Test-Path "version") {
        $BuildVersion = (Get-Content "version" -Raw).Trim()
    } else {
        $BuildVersion = "dev"
    }
}

# 检查管理员权限
$IsAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $IsAdmin) {
    Write-Host "[WARN] Not running as Administrator!" -ForegroundColor Yellow
    Write-Host "[WARN] VPN features may not work without administrator privileges." -ForegroundColor Yellow
    Write-Host ""
}

Write-Host ""
Write-Host "========================================"
Write-Host "Beagle Desktop - Development Mode"
Write-Host "========================================"
Write-Host "Version:           $BuildVersion"
Write-Host "Desktop Directory: $DesktopDir"
Write-Host "Admin Privileges:  $IsAdmin"
Write-Host ""

# 检查 Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] node command not found" -ForegroundColor Red
    Write-Host "Please install Node.js first: https://nodejs.org/"
    Read-Host "Press Enter to exit"
    exit 1
}

# 检查 Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] go command not found" -ForegroundColor Red
    Write-Host "Please install Go first: https://go.dev/"
    Read-Host "Press Enter to exit"
    exit 1
}

# 安装前端依赖（如果需要）
if (-not (Test-Path "frontend\node_modules")) {
    Write-Host "[INFO] Installing frontend dependencies..."
    Set-Location "frontend"
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to install frontend dependencies" -ForegroundColor Red
        Set-Location $DesktopDir
        Read-Host "Press Enter to exit"
        exit 1
    }
    Set-Location $DesktopDir
} else {
    Write-Host "[INFO] Frontend dependencies already installed"
}

# 确保 .tmp 目录存在
$TmpDir = ".tmp"
$TmpBinDir = ".tmp\bin"
if (-not (Test-Path $TmpDir)) {
    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
}
if (-not (Test-Path $TmpBinDir)) {
    New-Item -ItemType Directory -Path $TmpBinDir -Force | Out-Null
}

Write-Host ""
Write-Host "[INFO] Building frontend..."
Set-Location "frontend"
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Failed to build frontend" -ForegroundColor Red
    Set-Location $DesktopDir
    Read-Host "Press Enter to exit"
    exit 1
}
Set-Location $DesktopDir

Write-Host ""
Write-Host "[INFO] Building Go application..."
$OutputExe = "$TmpBinDir\signal_desktop.exe"
$BuildFlags = "-buildvcs=false -gcflags=all=-l"
$LdFlags = "-X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.Version=$BuildVersion'"

go build -o $OutputExe -ldflags $LdFlags -gcflags=all=-l .
$BuildResult = $LASTEXITCODE

if ($BuildResult -ne 0) {
    Write-Host "[ERROR] Failed to build Go application" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# 检查文件是否真的生成了
if (-not (Test-Path $OutputExe)) {
    Write-Host "[ERROR] Build reported success but executable not found: $OutputExe" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

Write-Host "[INFO] Build successful: $OutputExe" -ForegroundColor Green
Write-Host ""
Write-Host "[INFO] Starting application..."
if ($IsAdmin) {
    Write-Host "[INFO] Running with administrator privileges - system features enabled" -ForegroundColor Green
} else {
    Write-Host "[WARN] Not running as administrator - some features may not work!" -ForegroundColor Yellow
}
Write-Host ""

# 运行应用
& $OutputExe

$ExitCode = $LASTEXITCODE

Write-Host ""
Write-Host "[INFO] Application exited with code: $ExitCode"
Read-Host "Press Enter to exit"

exit $ExitCode
