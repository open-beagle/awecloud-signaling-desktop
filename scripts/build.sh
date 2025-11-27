#!/bin/bash

# Desktop 构建脚本

set -e

# Server 地址（可选，用于编译时注入）
BUILD_URL="${BUILD_URL:-}"

echo "Building AWECloud Desktop..."

if [ -n "${BUILD_URL}" ]; then
    echo "Build URL: ${BUILD_URL}"
fi

# 安装前端依赖
echo "Installing frontend dependencies..."
cd frontend
npm install
cd ..

# 构建 ldflags
LDFLAGS="-w -s"
if [ -n "${BUILD_URL}" ]; then
    LDFLAGS="${LDFLAGS} -X 'main.BUILD_URL=${BUILD_URL}'"
fi

# 构建应用
echo "Building application..."
if [ -n "${BUILD_URL}" ]; then
    wails build -clean -ldflags "${LDFLAGS}"
else
    wails build -clean
fi

echo "Build complete!"
echo "Output: build/bin/awecloud-desktop"
