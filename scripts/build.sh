#!/bin/bash

# Desktop 构建脚本

set -e

echo "Building AWECloud Desktop..."

# 安装前端依赖
echo "Installing frontend dependencies..."
cd frontend
npm install
cd ..

# 构建应用
echo "Building application..."
wails build -clean

echo "Build complete!"
echo "Output: build/bin/awecloud-desktop"
