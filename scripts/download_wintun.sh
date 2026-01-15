#!/bin/bash

# 下载 wintun.dll 用于嵌入构建
# 此脚本在 Windows 构建前自动执行

set -e

WINTUN_VERSION="0.14.1"
WINTUN_URL="https://www.wintun.net/builds/wintun-${WINTUN_VERSION}.zip"
TARGET_DIR="internal/tailscale/resources"
TARGET_FILE="${TARGET_DIR}/wintun.dll"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 检查是否已存在
if [ -f "${TARGET_FILE}" ]; then
    echo -e "${GREEN}✓ wintun.dll 已存在${NC}"
    exit 0
fi

echo -e "${YELLOW}下载 wintun ${WINTUN_VERSION}...${NC}"

# 创建临时目录
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

# 下载
curl -sL "${WINTUN_URL}" -o "${TMP_DIR}/wintun.zip"

# 解压
unzip -q "${TMP_DIR}/wintun.zip" -d "${TMP_DIR}"

# 复制 amd64 版本（Desktop 目前只支持 amd64）
mkdir -p "${TARGET_DIR}"
cp "${TMP_DIR}/wintun/bin/amd64/wintun.dll" "${TARGET_FILE}"

echo -e "${GREEN}✓ wintun.dll 已下载到 ${TARGET_FILE}${NC}"
