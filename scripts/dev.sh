#!/bin/bash

# Desktop 开发脚本

set -e

echo "Starting AWECloud Desktop in development mode..."

# 安装前端依赖（如果需要）
if [ ! -d "frontend/node_modules" ]; then
    echo "Installing frontend dependencies..."
    cd frontend
    npm install
    cd ..
fi

# 启动开发模式
wails dev
