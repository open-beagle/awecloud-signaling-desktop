# Desktop 开发脚本 - Windows PowerShell 版本（简化版）
# 工作目录: awecloud-signaling-server\

param(
    [string]$BuildVersion = $env:BUILD_VERSION
)

# 设置默认版本
if ([string]::IsNullOrEmpty($BuildVersion)) { $BuildVersion = "dev" }

# 检查管理员权限
$IsAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $IsAdmin) {
    Write-Host "[WARN] Not running as Administrator!" -ForegroundColor Yellow
    Write-Host "[WARN] VPN features may not work without administrator privileges." -ForegroundColor Yellow
    Write-Host ""
}

# 设置工作目录为项目根目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopDir = Split-Path -Parent $ScriptDir
$RootDir = Split-Path -Parent $DesktopDir
Set-Location $RootDir

Write-Host "========================================"
Write-Host "Beagle Desktop - Development Mode"
Write-Host "========================================"
Write-Host "Version:           $BuildVersion"
Write-Host "Root Directory:    $RootDir"
Write-Host "Desktop Directory: desktop\"
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
if (-not (Test-Path "desktop\frontend\node_modules")) {
    Write-Host "[INFO] Installing frontend dependencies..."
    Set-Location "desktop\frontend"
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to install frontend dependencies" -ForegroundColor Red
        Set-Location $RootDir
        Read-Host "Press Enter to exit"
        exit 1
    }
    Set-Location $RootDir
} else {
    Write-Host "[INFO] Frontend dependencies already installed"
}

# 确保 .tmp 目录存在
$TmpDir = "desktop\.tmp"
$TmpBinDir = "desktop\.tmp\bin"
if (-not (Test-Path $TmpDir)) {
    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
}
if (-not (Test-Path $TmpBinDir)) {
    New-Item -ItemType Directory -Path $TmpBinDir -Force | Out-Null
}

# 检查并下载 Wintun 驱动
$WintunVersion = "0.14.1"
$WintunTarget = "$TmpDir\wintun\bin\amd64\wintun.dll"

if (-not (Test-Path $WintunTarget)) {
    Write-Host "[INFO] Downloading wintun-$WintunVersion..."
    
    $WintunZip = "$TmpDir\wintun.zip"
    $WintunUrl = "https://www.wintun.net/builds/wintun-$WintunVersion.zip"
    
    try {
        Invoke-WebRequest -Uri $WintunUrl -OutFile $WintunZip -ErrorAction Stop
        
        Write-Host "[INFO] Extracting wintun..."
        Expand-Archive -Path $WintunZip -DestinationPath $TmpDir -Force
        Remove-Item $WintunZip -Force
        
        Write-Host "[INFO] Wintun downloaded successfully" -ForegroundColor Green
    } catch {
        Write-Host "[ERROR] Failed to download wintun: $_" -ForegroundColor Red
        Write-Host "[WARN] VPN features will not work." -ForegroundColor Yellow
    }
} else {
    Write-Host "[INFO] Wintun already exists: $WintunTarget"
}

Write-Host ""
Write-Host "[INFO] Building frontend..."
Set-Location "desktop\frontend"
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Failed to build frontend" -ForegroundColor Red
    Set-Location $RootDir
    Read-Host "Press Enter to exit"
    exit 1
}
Set-Location $RootDir

Write-Host ""
Write-Host "[INFO] Building Go application..."
$OutputExe = "$TmpBinDir\awecloud-signaling-desktop.exe"
$BuildFlags = "-buildvcs=false -gcflags=all=-l"
$LdFlags = "-X 'github.com/open-beagle/awecloud-signaling-desktop/internal/version.Version=$BuildVersion'"

Set-Location "desktop"
go build -o "..\$OutputExe" -ldflags $LdFlags -gcflags=all=-l .
$BuildResult = $LASTEXITCODE
Set-Location $RootDir

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
    Write-Host "[INFO] Running with administrator privileges - VPN features enabled" -ForegroundColor Green
} else {
    Write-Host "[WARN] Not running as administrator - VPN features may not work!" -ForegroundColor Yellow
}
Write-Host ""

# 运行应用（使用完整路径避免 PowerShell 模块加载错误）
$FullExePath = Join-Path $RootDir $OutputExe
& $FullExePath

$ExitCode = $LASTEXITCODE

Write-Host ""
Write-Host "[INFO] Application exited with code: $ExitCode"
Read-Host "Press Enter to exit"

exit $ExitCode
