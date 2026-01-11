@echo off
setlocal enabledelayedexpansion

REM Desktop 开发脚本 - Windows 版本

REM 切换到项目根目录（脚本所在目录的上一级）
cd /d "%~dp0.."

REM 设置默认版本
if "%BUILD_VERSION%"=="" set BUILD_VERSION=v0.2.0

echo ========================================
echo AWECloud Desktop - Development Mode
echo ========================================
echo Version: %BUILD_VERSION%
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

echo.
echo [INFO] Starting Wails development server...
echo.

REM 设置版本环境变量供应用读取
set APP_VERSION=%BUILD_VERSION%

REM 启动开发模式
wails3 dev
