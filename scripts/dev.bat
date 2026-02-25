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

REM 读取版本号
if "%BUILD_VERSION%"=="" (
    if exist "version" (
        set /p BUILD_VERSION=<version
    ) else (
        set BUILD_VERSION=v0.2.0
    )
)

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

echo.
echo [INFO] Starting Wails development server...
echo [WARN] ENSURE YOU ARE RUNNING AS ADMINISTRATOR for system-level features!
echo.

REM 设置版本环境变量供应用读取
set APP_VERSION=%BUILD_VERSION%

REM 启动开发模式（使用 -port 参数避开 Windows 保留端口 9200-9299）
wails3 dev -port 34115 < nul
