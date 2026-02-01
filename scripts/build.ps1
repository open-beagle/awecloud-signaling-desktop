# Desktop Build Script (Wails v3) - Windows PowerShell
# Working Directory: awecloud-signaling-server\

param(
    [string]$BuildVersion = $env:BUILD_VERSION,
    [string]$BuildAddress = $env:BUILD_ADDRESS,
    [string]$GoArch = $env:GOARCH
)

# Set defaults
if ([string]::IsNullOrEmpty($BuildVersion)) { $BuildVersion = "dev" }
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

# Set working directory to project root
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DesktopDir = Split-Path -Parent $ScriptDir
$RootDir = Split-Path -Parent $DesktopDir
Set-Location $RootDir

$OutputDir = "desktop\build\bin"

Write-Host "========================================"
Write-Host "AWECloud Signaling Desktop Builder"
Write-Host "========================================"
Write-Host ""
Write-Host "Root Directory: $RootDir"
Write-Host "Version:        $BuildVersion"
Write-Host "Build Number:   $BuildNumber"
Write-Host "Address:        $BuildAddress"
Write-Host "Git Commit:     $GitCommit"
Write-Host "Build Date:     $BuildDate"
Write-Host "Architecture:   $GoArch"
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
Set-Location "desktop\frontend"
if (-not (Test-Path "node_modules")) {
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to install frontend dependencies" -ForegroundColor Red
        Set-Location $RootDir
        exit 1
    }
} else {
    Write-Host "Frontend dependencies already installed, skipping..."
}
Set-Location $RootDir

# Build frontend
Write-Host "[INFO] Building frontend..."
Set-Location "desktop\frontend"
npm run build
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Failed to build frontend" -ForegroundColor Red
    Set-Location $RootDir
    exit 1
}
Set-Location $RootDir

# Generate bindings
if ($Wails3Available) {
    Write-Host "[INFO] Generating bindings..."
    Set-Location "desktop"
    wails3 generate bindings
    Set-Location $RootDir
} else {
    if (Test-Path "desktop\frontend\bindings") {
        Write-Host "[INFO] wails3 not available, using existing bindings..."
    } else {
        Write-Host "[ERROR] wails3 not available and no existing bindings found" -ForegroundColor Red
        Write-Host "Please install wails3 or ensure desktop\frontend\bindings directory exists"
        exit 1
    }
}

# Create output directory
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

# Download and prepare Wintun for embedding
$WintunVersion = "0.14.1"
$WintunResourceDir = "desktop\internal\tailscale\resources"
$WintunDllPath = "$WintunResourceDir\wintun.dll"

if (-not (Test-Path $WintunResourceDir)) {
    New-Item -ItemType Directory -Path $WintunResourceDir -Force | Out-Null
}

if (-not (Test-Path $WintunDllPath)) {
    Write-Host "[INFO] Downloading wintun-$WintunVersion for embedding..."
    
    $TmpDir = "desktop\.tmp"
    if (-not (Test-Path $TmpDir)) {
        New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
    }
    
    $WintunZip = "$TmpDir\wintun.zip"
    $WintunUrl = "https://www.wintun.net/builds/wintun-$WintunVersion.zip"
    
    try {
        Invoke-WebRequest -Uri $WintunUrl -OutFile $WintunZip -ErrorAction Stop
        
        Write-Host "[INFO] Extracting wintun..."
        Expand-Archive -Path $WintunZip -DestinationPath $TmpDir -Force
        Remove-Item $WintunZip -Force
        
        $WintunFound = $false
        $PossibleDirs = @("$TmpDir\wintun", "$TmpDir\wintun-$WintunVersion")
        
        foreach ($Dir in $PossibleDirs) {
            $SourceDll = "$Dir\bin\amd64\wintun.dll"
            if (Test-Path $SourceDll) {
                Copy-Item $SourceDll $WintunDllPath -Force
                Write-Host "[SUCCESS] Copied wintun.dll to $WintunResourceDir" -ForegroundColor Green
                $WintunFound = $true
                break
            }
        }
        
        if (-not $WintunFound) {
            Write-Host "[ERROR] wintun.dll not found after extraction!" -ForegroundColor Red
            exit 1
        }
    } catch {
        Write-Host "[ERROR] Failed to download wintun: $_" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "[INFO] Wintun already exists for embedding: $WintunDllPath"
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

Set-Location "desktop"

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
    Set-Location $RootDir
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

$BuildOutput = "build\bin\awecloud-signaling-desktop.exe"
$OutputName = "awecloud-signaling-$BuildVersion-windows-$GoArch.exe"

Write-Host "Building with: go build -tags production -trimpath -ldflags `"$LdFlags`" -o $BuildOutput"
go build -tags production -trimpath -ldflags $LdFlags -o $BuildOutput

if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Build failed" -ForegroundColor Red
    Remove-Item "rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
    Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue
    Set-Location $RootDir
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
    Set-Location $RootDir
    exit 1
}

# Cleanup
Remove-Item "rsrc_windows_*.syso" -Force -ErrorAction SilentlyContinue
Remove-Item "winres" -Recurse -Force -ErrorAction SilentlyContinue

Set-Location $RootDir

Write-Host ""
Write-Host "========================================"
Write-Host "Build completed successfully!"
Write-Host "========================================"
Write-Host ""
Write-Host "Output directory: $OutputDir"
Get-ChildItem $OutputDir

exit 0
