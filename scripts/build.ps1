# Desktop Build Script (Wails v3) - Windows PowerShell
# Working Directory: awecloud-signaling-server\desktop\

param(
    [string]$BuildVersion = $env:BUILD_VERSION,
    [string]$GoArch = $env:GOARCH
)

# Set working directory to desktop folder
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopDir = Split-Path -Parent $ScriptDir
Set-Location $DesktopDir

# Local and CI packaging both read .env. .env.example is only a template.
$EnvPath = Join-Path $DesktopDir ".env"
if (-not (Test-Path -LiteralPath $EnvPath)) {
    Write-Host "[ERROR] desktop/.env is required for packaging" -ForegroundColor Red
    exit 1
}

$EnvConfig = @{}
Get-Content -LiteralPath $EnvPath | ForEach-Object {
    $Line = $_.Trim()
    if ($Line -eq "" -or $Line.StartsWith("#")) { return }
    $Pair = $Line.Split("=", 2)
    if ($Pair.Count -eq 2) {
        $EnvConfig[$Pair[0].Trim()] = $Pair[1].Trim()
    }
}
$BuildAddress = $EnvConfig["SIGNALING_ADDRESS"]
if ([string]::IsNullOrWhiteSpace($BuildAddress)) {
    Write-Host "[ERROR] SIGNALING_ADDRESS is required in desktop/.env" -ForegroundColor Red
    exit 1
}

# Read version from file if not provided
if ([string]::IsNullOrEmpty($BuildVersion)) {
    if (Test-Path "version") {
        $BuildVersion = (Get-Content "version" -Raw).Trim()
    } else {
        $BuildVersion = "dev"
    }
}
if ([string]::IsNullOrEmpty($GoArch)) { $GoArch = "amd64" }

# Get Git info
try {
    $GitCommit = git rev-parse --short HEAD 2>$null
    if ([string]::IsNullOrEmpty($GitCommit)) { $GitCommit = "unknown" }
} catch {
    $GitCommit = "unknown"
}

try {
    $BuildNumber = git rev-list --count HEAD 2>$null
    if ([string]::IsNullOrEmpty($BuildNumber)) { $BuildNumber = "0" }
} catch {
    $BuildNumber = "0"
}

$BuildDate = Get-Date -Format "yyyy-MM-dd_HH:mm:ss"

$OutputDir = "build\bin"

Write-Host "========================================"
Write-Host "AWECloud Signaling Desktop Builder"
Write-Host "========================================"
Write-Host ""
Write-Host "Desktop Directory: $DesktopDir"
Write-Host "Version:           $BuildVersion"
Write-Host "Build Number:      $BuildNumber"
Write-Host "Environment:       .env loaded"
Write-Host "Git Commit:        $GitCommit"
Write-Host "Build Date:        $BuildDate"
Write-Host "Architecture:      $GoArch"
Write-Host ""

# Check Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] node command not found" -ForegroundColor Red
    Write-Host "Please install Node.js first: https://nodejs.org/"
    exit 1
}

# Check Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] go command not found" -ForegroundColor Red
    Write-Host "Please install Go first: https://go.dev/"
    exit 1
}

# Install frontend dependencies
Write-Host "[INFO] Installing frontend dependencies..."
Set-Location "frontend"
if (-not (Test-Path "node_modules")) {
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to install frontend dependencies" -ForegroundColor Red
        Set-Location $DesktopDir
        exit 1
    }
} else {
    Write-Host "Frontend dependencies already installed, skipping..."
}
Set-Location $DesktopDir

# Release and local builds use the bindings committed for the exact Wails
# runtime in go.mod. A globally installed wails3 may be a different version
# and must not replace these files during packaging.
if (-not (Test-Path "frontend\bindings")) {
    Write-Host "[ERROR] frontend\bindings is required" -ForegroundColor Red
    Write-Host "Regenerate it with the Wails version pinned in go.mod before packaging"
    exit 1
}
Write-Host "[INFO] Using committed frontend bindings..."

# Build frontend
Write-Host "[INFO] Building frontend..."
Set-Location "frontend"
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Failed to build frontend" -ForegroundColor Red
    Set-Location $DesktopDir
    exit 1
}
Set-Location $DesktopDir

# Create output directory
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

# Generate Windows resources
Write-Host "[INFO] Generating Windows resources (icon embedding)..."

# Check go-winres
if (-not (Get-Command go-winres -ErrorAction SilentlyContinue)) {
    Write-Host "[INFO] Installing go-winres..."
    go install github.com/tc-hib/go-winres@latest
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to install go-winres" -ForegroundColor Red
        exit 1
    }
}

# Create winres directory
if (-not (Test-Path "winres")) {
    New-Item -ItemType Directory -Path "winres" -Force | Out-Null
}

# Copy icon files
if (Test-Path "build\windows\icon.ico") {
    Copy-Item "build\windows\icon.ico" "winres\icon.ico" -Force
} elseif (Test-Path "build\appicon.png") {
    Copy-Item "build\appicon.png" "winres\icon.png" -Force
}

# Generate manifest file
$ManifestXml = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
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
</assembly>'

$ManifestXml | Out-File -FilePath "winres\app.manifest" -Encoding ascii -NoNewline

# Generate winres.json config
$WinresJson = @"
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
                    "file_version": "$BuildVersion.$BuildNumber",
                    "product_version": "$BuildVersion.$BuildNumber"
                },
                "info": {
                    "0409": {
                        "CompanyName": "AWECloud",
                        "FileDescription": "AWECloud Signaling Desktop",
                        "FileVersion": "$BuildVersion",
                        "InternalName": "awecloud-signaling-desktop",
                        "LegalCopyright": "Copyright (c) 2025 AWECloud. All rights reserved.",
                        "OriginalFilename": "awecloud-signaling-desktop.exe",
                        "ProductName": "Signaling Desktop",
                        "ProductVersion": "$BuildVersion"
                    }
                }
            }
        }
    }
}
"@

$WinresJson | Out-File -FilePath "winres\winres.json" -Encoding ascii

# Go only links .syso files from the package being built. Generate one for
# each Windows executable so both Desktop and Launcher receive the icon.
foreach ($ResourcePrefix in @("cmd\desktop\rsrc", "cmd\launcher\rsrc")) {
    Write-Host "Running: go-winres make --arch $GoArch --out $ResourcePrefix"
    go-winres make --arch $GoArch --out $ResourcePrefix
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path "${ResourcePrefix}_windows_$GoArch.syso")) {
        Write-Host "[ERROR] Failed to generate Windows resources: $ResourcePrefix" -ForegroundColor Red
        exit 1
    }
}
Write-Host "[SUCCESS] Windows resources generated for Desktop and Launcher" -ForegroundColor Green

# Build
Write-Host ""
Write-Host "========================================"
Write-Host "Building for windows/$GoArch"
Write-Host "========================================"

$env:GOOS = "windows"
$env:CGO_ENABLED = "0"
$env:GOARCH = $GoArch

# Build ldflags
$LdFlags = "-w -s -H windowsgui"
$LdFlags += " -X `"github.com/open-beagle/awecloud-signaling-desktop/internal/version.Version=$BuildVersion`""
$LdFlags += " -X `"github.com/open-beagle/awecloud-signaling-desktop/internal/version.GitCommit=$GitCommit`""
$LdFlags += " -X `"github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildTime=$BuildDate`""
$LdFlags += " -X `"github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildNumber=$BuildNumber`""
if (-not [string]::IsNullOrEmpty($BuildAddress)) {
    $LdFlags += " -X `"github.com/open-beagle/awecloud-signaling-desktop/internal/config.buildAddress=$BuildAddress`""
}

$BuildOutput = "build\bin\awecloud-signaling-desktop.exe"
$BuildOutputFull = [System.IO.Path]::GetFullPath((Join-Path $DesktopDir $BuildOutput))
$BuildBackup = "$BuildOutputFull~"
$LauncherOutput = "build\bin\beagle-signal.launcher.exe"
$LauncherOutputFull = [System.IO.Path]::GetFullPath((Join-Path $DesktopDir $LauncherOutput))

function Remove-WindowsResources {
    Remove-Item "cmd\desktop\rsrc_windows_*.syso", "cmd\launcher\rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
    Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue
}

# Windows may rename an in-use output to .exe~ and leave a second artifact.
# Refuse to build over the running fixed output so every successful build has
# exactly one canonical filename and location.
$RunningBuild = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Where-Object { $_.ExecutablePath -and ([System.IO.Path]::GetFullPath($_.ExecutablePath) -eq $BuildOutputFull) }
if ($RunningBuild) {
    $RunningPids = ($RunningBuild | Select-Object -ExpandProperty ProcessId) -join ", "
    Write-Host "[ERROR] Fixed output is running (PID: $RunningPids): $BuildOutputFull" -ForegroundColor Red
    Write-Host "Close the Desktop process before rebuilding." -ForegroundColor Yellow
    Remove-WindowsResources
    exit 1
}

if (Test-Path -LiteralPath $BuildBackup) {
    Remove-Item -LiteralPath $BuildBackup -Force
}

Write-Host "Building Desktop binary: $BuildOutput"
go build -tags production -trimpath -ldflags $LdFlags -o $BuildOutput ./cmd/desktop

if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Build failed" -ForegroundColor Red
    Remove-WindowsResources
    exit 1
}

$RunningLauncher = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
    Where-Object { $_.ExecutablePath -and ([System.IO.Path]::GetFullPath($_.ExecutablePath) -eq $LauncherOutputFull) }
if ($RunningLauncher) {
    $RunningPids = ($RunningLauncher | Select-Object -ExpandProperty ProcessId) -join ", "
    Write-Host "[ERROR] Launcher output is running (PID: $RunningPids): $LauncherOutputFull" -ForegroundColor Red
    Write-Host "Close the Launcher before rebuilding." -ForegroundColor Yellow
    Remove-WindowsResources
    exit 1
}

Write-Host "Building Launcher binary: $LauncherOutput"
go build -tags production -trimpath -ldflags $LdFlags -o $LauncherOutput ./cmd/launcher
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Launcher build failed" -ForegroundColor Red
    Remove-WindowsResources
    exit 1
}

# Check build result
if (Test-Path $BuildOutput) {
    if (Test-Path -LiteralPath $BuildBackup) {
        Write-Host "[ERROR] Unexpected backup artifact created: $BuildBackup" -ForegroundColor Red
        Remove-WindowsResources
        exit 1
    }
    Write-Host "[SUCCESS] Build successful: $BuildOutput" -ForegroundColor Green

    $FileSize = (Get-Item $BuildOutput).Length
    Write-Host "  File size: $FileSize bytes"

    if (-not (Test-Path $LauncherOutput)) {
        Write-Host "[ERROR] Launcher output file not found" -ForegroundColor Red
        Remove-WindowsResources
        exit 1
    }
    $LauncherFileSize = (Get-Item $LauncherOutput).Length
    Write-Host "[SUCCESS] Launcher build successful: $LauncherOutput" -ForegroundColor Green
    Write-Host "  File size: $LauncherFileSize bytes"
} else {
    Write-Host "[ERROR] Build failed - output file not found" -ForegroundColor Red
    Remove-WindowsResources
    exit 1
}

# Cleanup
Remove-WindowsResources

Write-Host ""
Write-Host "========================================"
Write-Host "Build completed successfully!"
Write-Host "========================================"
Write-Host ""
Write-Host "Output directory: $OutputDir"
Get-ChildItem $OutputDir

exit 0
