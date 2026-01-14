@echo off
setlocal enabledelayedexpansion

REM Desktop 开发脚本 - Windows 版本
REM 工作目录: desktop\

REM ----------------------------------------------------------------
REM 自动获取管理员权限 (System VPN 需要)
REM ----------------------------------------------------------------
>nul 2>&1 "%SYSTEMROOT%\system32\cacls.exe" "%SYSTEMROOT%\system32\config\system"
if '%errorlevel%' NEQ '0' (
    echo [INFO] Requesting administrative privileges...
    goto UACPrompt
) else ( goto gotAdmin )

:UACPrompt
    echo Set UAC = CreateObject^("Shell.Application"^) > "%temp%\getadmin.vbs"
    echo UAC.ShellExecute "%~s0", "", "", "runas", 1 >> "%temp%\getadmin.vbs"
    "%temp%\getadmin.vbs"
    exit /B

:gotAdmin
    if exist "%temp%\getadmin.vbs" ( del "%temp%\getadmin.vbs" )
    pushd "%CD%"
    REM 切换到 desktop 目录（脚本所在目录的上级）
    CD /D "%~dp0.."

REM ----------------------------------------------------------------

REM 设置默认版本
if "%BUILD_VERSION%"=="" set BUILD_VERSION=v0.2.0

echo ========================================
echo Beagle Desktop - Development Mode
echo ========================================
echo Version: %BUILD_VERSION%
echo Working Directory: %CD%
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

REM 检查 wails3 是否安装
where wails3 >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] wails3 command not found
    echo Please install Wails v3 first: go install github.com/wailsapp/wails/v3/cmd/wails3@latest
    exit /b 1
)

REM 安装前端依赖（如果需要）
if not exist "frontend\node_modules" (
    echo [INFO] Installing frontend dependencies...
    cd frontend
    call npm install
    if %ERRORLEVEL% neq 0 (
        echo [ERROR] Failed to install frontend dependencies
        cd ..
        exit /b 1
    )
    cd ..
) else (
    echo [INFO] Frontend dependencies already installed
)

REM ----------------------------------------------------------------
REM 确保 .tmp 目录存在
REM ----------------------------------------------------------------
if not exist ".tmp" (
    mkdir ".tmp"
)
if not exist ".tmp\bin" (
    mkdir ".tmp\bin"
)

REM ----------------------------------------------------------------
REM 检查并下载 Wintun 驱动
REM ----------------------------------------------------------------
set WINTUN_VERSION=0.14.1
set WINTUN_TARGET=build\bin\wintun.dll

REM 确保 build\bin 目录存在
if not exist "build\bin" (
    mkdir "build\bin"
)

if not exist "%WINTUN_TARGET%" (
    echo [INFO] Downloading wintun-%WINTUN_VERSION%...
    
    REM 下载到 .tmp 目录
    powershell -Command "Invoke-WebRequest -Uri 'https://www.wintun.net/builds/wintun-%WINTUN_VERSION%.zip' -OutFile '.tmp\wintun.zip'"
    
    if exist ".tmp\wintun.zip" (
        echo [INFO] Extracting wintun...
        powershell -Command "Expand-Archive -Path '.tmp\wintun.zip' -DestinationPath '.tmp' -Force"
        del ".tmp\wintun.zip"
        
        REM 查找并复制 wintun.dll（支持多种解压目录名）
        set WINTUN_FOUND=0
        for %%D in (".tmp\wintun" ".tmp\wintun-%WINTUN_VERSION%") do (
            if exist "%%~D\bin\amd64\wintun.dll" (
                copy /Y "%%~D\bin\amd64\wintun.dll" "%WINTUN_TARGET%" >nul
                echo [INFO] Copied wintun.dll to build\bin\
                set WINTUN_FOUND=1
            )
        )
        
        if "!WINTUN_FOUND!"=="0" (
            echo [ERROR] wintun.dll not found after extraction!
            echo [ERROR] Please check .tmp directory structure
        )
    ) else (
        echo [ERROR] Failed to download wintun. VPN features will not work.
    )
) else (
    echo [INFO] Wintun already exists: %WINTUN_TARGET%
)

echo.
echo [INFO] Starting Wails development server...
echo [WARN] ENSURE YOU ARE RUNNING AS ADMINISTRATOR for VPN features!
echo.

REM 设置版本环境变量供应用读取
set APP_VERSION=%BUILD_VERSION%

REM 启动开发模式（使用 -port 参数避开 Windows 保留端口 9200-9299）
wails3 dev -port 34115 < nul
