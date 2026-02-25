# Desktop Build Script (Wails v3) - Windows PowerShell
# Working Directory: awecloud-signaling-server\desktop\

param(
    [string]$BuildVersion = $env:BUILD_VERSION,
    [string]$BuildAddress = $env:BUILD_ADDRESS,
    [string]$GoArch = $env:GOARCH
)

# Set working directory to desktop folder
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopDir = Split-Path -Parent $ScriptDir
Set-Location $DesktopDir

# Read version from file if not provided
if ([string]::IsNullOrEmpty($BuildVersion)) {
    if (Test-Path "version") {
        $BuildVersion = (Get-Content "version" -Raw).Trim()
    } else {
        $BuildVersion = "dev"
    }
}
if ([string]::IsNullOrEmpty($BuildAddress)) { 
    # 尝试从 SIGNALING_ADDRESS 环境变量读取（与 Linux 版本保持一致）
    $BuildAddress = $env:SIGNALING_ADDRESS
    if ([string]::IsNullOrEmpty($BuildAddress)) { 
        $BuildAddress = "" 
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
Write-Host "Address:           $BuildAddress"
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

# Check wails3
$Wails3Available = $false
if (Get-Command wails3 -ErrorAction SilentlyContinue) {
    $Wails3Available = $true
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

# Generate bindings (必须在构建前端之前)
if ($Wails3Available) {
    Write-Host "[INFO] Generating bindings..."
    wails3 generate bindings
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to generate bindings" -ForegroundColor Red
        exit 1
    }
} else {
    if (Test-Path "frontend\bindings") {
        Write-Host "[INFO] wails3 not available, using existing bindings..."
    } else {
        Write-Host "[ERROR] wails3 not available and no existing bindings found" -ForegroundColor Red
        Write-Host "Please install wails3 or ensure frontend\bindings directory exists"
        exit 1
    }
}

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

# Generate .syso file
Write-Host "Running: go-winres make --arch $GoArch"
go-winres make --arch $GoArch

if (Test-Path "rsrc_windows_$GoArch.syso") {
    Write-Host "[SUCCESS] Windows resources generated: rsrc_windows_$GoArch.syso" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Failed to generate Windows resources" -ForegroundColor Red
    exit 1
}

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

$BuildOutput = "build\bin\signal_desktop.exe"
$OutputName = "signal_desktop-$BuildVersion-windows-$GoArch.exe"

Write-Host "Building with: go build -tags production -trimpath -ldflags `"$LdFlags`" -o $BuildOutput"
go build -tags production -trimpath -ldflags $LdFlags -o $BuildOutput

if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Build failed" -ForegroundColor Red
    Remove-Item "rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
    Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

# Check build result
if (Test-Path $BuildOutput) {
    Write-Host "[SUCCESS] Build successful: $BuildOutput" -ForegroundColor Green
    
    Copy-Item $BuildOutput "build\bin\$OutputName" -Force
    Write-Host "[SUCCESS] Output: build\bin\$OutputName" -ForegroundColor Green
    
    $FileSize = (Get-Item "build\bin\$OutputName").Length
    Write-Host "  File size: $FileSize bytes"
} else {
    Write-Host "[ERROR] Build failed - output file not found" -ForegroundColor Red
    Remove-Item "rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
    Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

# Cleanup
Remove-Item "rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "========================================"
Write-Host "Build completed successfully!"
Write-Host "========================================"
Write-Host ""
Write-Host "Output directory: $OutputDir"
Get-ChildItem $OutputDir

exit 0
