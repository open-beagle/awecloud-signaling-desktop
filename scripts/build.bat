@echo off
setlocal enabledelayedexpansion

REM Desktop 构建脚本 (Wails v3) - Windows 版本

REM 版本信息
if "%BUILD_VERSION%"=="" set BUILD_VERSION=v0.2.0
if "%BUILD_ADDRESS%"=="" set BUILD_ADDRESS=

REM 获取 Git 信息
for /f "tokens=*" %%i in ('git rev-parse --short HEAD 2^>nul') do set GIT_COMMIT=%%i
if "%GIT_COMMIT%"=="" set GIT_COMMIT=unknown

for /f "tokens=*" %%i in ('git rev-list --count HEAD 2^>nul') do set BUILD_NUMBER=%%i
if "%BUILD_NUMBER%"=="" set BUILD_NUMBER=0

for /f "tokens=*" %%i in ('powershell -command "Get-Date -Format 'yyyy-MM-dd_HH:mm:ss'"') do set BUILD_DATE=%%i

REM 目标架构
if "%GOARCH%"=="" set GOARCH=amd64

set OUTPUT_DIR=.\build\bin

echo ========================================
echo AWECloud Signaling Desktop Builder
echo ========================================
echo.
echo Version:      %BUILD_VERSION%
echo Build Number: %BUILD_NUMBER%
echo Address:      %BUILD_ADDRESS%
echo Git Commit:   %GIT_COMMIT%
echo Build Date:   %BUILD_DATE%
echo Architecture: %GOARCH%
echo.

REM 检查 Node.js 是否安装
where node >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] node command not found
    echo Please install Node.js first: https://nodejs.org/
    exit /b 1
)

REM 检查 Go 是否安装
where go >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] go command not found
    echo Please install Go first: https://go.dev/
    exit /b 1
)

REM 检查 wails3 是否可用
set WAILS3_AVAILABLE=0
where wails3 >nul 2>nul
if %ERRORLEVEL% equ 0 set WAILS3_AVAILABLE=1

REM 安装前端依赖
echo [INFO] Installing frontend dependencies...
cd frontend
if not exist "node_modules" (
    call npm install
    if %ERRORLEVEL% neq 0 (
        echo [ERROR] Failed to install frontend dependencies
        cd ..
        exit /b 1
    )
) else (
    echo Frontend dependencies already installed, skipping...
)
cd ..

REM 构建前端
echo [INFO] Building frontend...
cd frontend
call npm run build
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Failed to build frontend
    cd ..
    exit /b 1
)
cd ..

REM 生成绑定
if %WAILS3_AVAILABLE% equ 1 (
    echo [INFO] Generating bindings...
    wails3 generate bindings
) else (
    if exist "frontend\bindings" (
        echo [INFO] wails3 not available, using existing bindings...
    ) else (
        echo [ERROR] wails3 not available and no existing bindings found
        echo Please install wails3 or ensure frontend\bindings directory exists
        exit /b 1
    )
)

REM 创建输出目录
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

REM 生成 Windows 资源文件
call :generateWindowsResources
if %ERRORLEVEL% neq 0 exit /b 1

REM 构建
echo.
echo ========================================
echo Building for windows/%GOARCH%
echo ========================================

set GOOS=windows
set CGO_ENABLED=0

REM 构建 ldflags
set LDFLAGS=-w -s -H windowsgui
set LDFLAGS=%LDFLAGS% -X "github.com/open-beagle/awecloud-signaling-desktop/internal/version.Version=%BUILD_VERSION%"
set LDFLAGS=%LDFLAGS% -X "github.com/open-beagle/awecloud-signaling-desktop/internal/version.GitCommit=%GIT_COMMIT%"
set LDFLAGS=%LDFLAGS% -X "github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildTime=%BUILD_DATE%"
set LDFLAGS=%LDFLAGS% -X "github.com/open-beagle/awecloud-signaling-desktop/internal/version.BuildNumber=%BUILD_NUMBER%"
if not "%BUILD_ADDRESS%"=="" (
    set LDFLAGS=!LDFLAGS! -X "github.com/open-beagle/awecloud-signaling-desktop/internal/config.buildAddress=%BUILD_ADDRESS%"
)

set BUILD_OUTPUT=%OUTPUT_DIR%\awecloud-signaling-desktop.exe
set OUTPUT_NAME=awecloud-signaling-%BUILD_VERSION%-windows-%GOARCH%.exe

echo Building with: go build -tags production -trimpath -ldflags "%LDFLAGS%" -o %BUILD_OUTPUT%
go build -tags production -trimpath -ldflags "%LDFLAGS%" -o %BUILD_OUTPUT%

if %ERRORLEVEL% neq 0 (
    echo [ERROR] Build failed
    call :cleanWindowsResources
    exit /b 1
)

REM 检查构建结果
if exist "%BUILD_OUTPUT%" (
    echo [SUCCESS] Build successful: %BUILD_OUTPUT%
    
    REM 复制并重命名
    copy "%BUILD_OUTPUT%" "%OUTPUT_DIR%\%OUTPUT_NAME%" >nul
    echo [SUCCESS] Output: %OUTPUT_DIR%\%OUTPUT_NAME%
    
    REM 显示文件大小
    for %%A in ("%OUTPUT_DIR%\%OUTPUT_NAME%") do echo   File size: %%~zA bytes
) else (
    echo [ERROR] Build failed - output file not found
    call :cleanWindowsResources
    exit /b 1
)

REM 清理资源文件
call :cleanWindowsResources

echo.
echo ========================================
echo Build completed successfully!
echo ========================================
echo.
echo Output directory: %OUTPUT_DIR%\
dir "%OUTPUT_DIR%"

exit /b 0

REM ========================================
REM 函数定义
REM ========================================

:generateWindowsResources
echo [INFO] Generating Windows resources (icon embedding)...

REM 检查 go-winres 是否安装
where go-winres >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [INFO] Installing go-winres...
    go install github.com/tc-hib/go-winres@latest
    if %ERRORLEVEL% neq 0 (
        echo [ERROR] Failed to install go-winres
        exit /b 1
    )
)

REM 创建 winres 目录
if not exist "winres" mkdir winres

REM 复制图标文件
if exist "build\windows\icon.ico" (
    copy "build\windows\icon.ico" "winres\icon.ico" >nul
) else if exist "build\appicon.png" (
    copy "build\appicon.png" "winres\icon.png" >nul
)

REM 生成 manifest 文件
(
echo ^<?xml version="1.0" encoding="UTF-8" standalone="yes"?^>
echo ^<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3"^>
echo     ^<assemblyIdentity type="win32" name="com.awecloud.signaling.desktop" version="1.0.0.0" processorArchitecture="*"/^>
echo     ^<dependency^>
echo         ^<dependentAssembly^>
echo             ^<assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/^>
echo         ^</dependentAssembly^>
echo     ^</dependency^>
echo     ^<asmv3:application^>
echo         ^<asmv3:windowsSettings^>
echo             ^<dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings"^>true/pm^</dpiAware^>
echo             ^<dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings"^>permonitorv2,permonitor^</dpiAwareness^>
echo         ^</asmv3:windowsSettings^>
echo     ^</asmv3:application^>
echo ^</assembly^>
) > winres\app.manifest

REM 生成 winres.json 配置文件
(
echo {
echo     "RT_GROUP_ICON": {
echo         "APP": {
echo             "0000": "icon.ico"
echo         }
echo     },
echo     "RT_MANIFEST": {
echo         "#1": {
echo             "0000": "app.manifest"
echo         }
echo     },
echo     "RT_VERSION": {
echo         "#1": {
echo             "0000": {
echo                 "fixed": {
echo                     "file_version": "%BUILD_VERSION%.%BUILD_NUMBER%",
echo                     "product_version": "%BUILD_VERSION%.%BUILD_NUMBER%"
echo                 },
echo                 "info": {
echo                     "0409": {
echo                         "CompanyName": "AWECloud",
echo                         "FileDescription": "AWECloud Signaling Desktop",
echo                         "FileVersion": "%BUILD_VERSION%",
echo                         "InternalName": "awecloud-signaling-desktop",
echo                         "LegalCopyright": "Copyright © 2025 AWECloud. All rights reserved.",
echo                         "OriginalFilename": "awecloud-signaling-desktop.exe",
echo                         "ProductName": "Signaling Desktop",
echo                         "ProductVersion": "%BUILD_VERSION%"
echo                     }
echo                 }
echo             }
echo         }
echo     }
echo }
) > winres\winres.json

REM 生成 .syso 文件
echo Running: go-winres make --arch %GOARCH%
go-winres make --arch %GOARCH%

if exist "rsrc_windows_%GOARCH%.syso" (
    echo [SUCCESS] Windows resources generated: rsrc_windows_%GOARCH%.syso
) else (
    echo [ERROR] Failed to generate Windows resources
    exit /b 1
)

exit /b 0

:cleanWindowsResources
if exist "rsrc_windows_*.syso" del /q rsrc_windows_*.syso 2>nul
if exist "winres" rmdir /s /q winres 2>nul
exit /b 0
