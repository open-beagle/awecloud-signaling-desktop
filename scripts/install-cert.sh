#!/bin/bash
# 安装自签名证书到系统信任列表

set -e

CERT_FILE="$1"

if [ -z "$CERT_FILE" ]; then
    echo "用法: $0 <证书文件路径>"
    echo "示例: $0 server.crt"
    exit 1
fi

if [ ! -f "$CERT_FILE" ]; then
    echo "错误：证书文件不存在: $CERT_FILE"
    exit 1
fi

echo "正在安装证书: $CERT_FILE"

if [ "$(uname)" == "Darwin" ]; then
    # macOS
    echo "检测到 macOS 系统"
    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$CERT_FILE"
    echo "✓ 证书已成功安装到 macOS 系统钥匙串"
    echo "请重启 Desktop 应用以使更改生效"
    
elif [ "$(uname)" == "Linux" ]; then
    # Linux
    echo "检测到 Linux 系统"
    
    # 检测发行版
    if [ -f /etc/debian_version ]; then
        # Debian/Ubuntu
        echo "检测到 Debian/Ubuntu 系统"
        sudo cp "$CERT_FILE" /usr/local/share/ca-certificates/
        sudo update-ca-certificates
        echo "✓ 证书已成功安装到系统信任列表"
        
    elif [ -f /etc/redhat-release ]; then
        # CentOS/RHEL
        echo "检测到 CentOS/RHEL 系统"
        sudo cp "$CERT_FILE" /etc/pki/ca-trust/source/anchors/
        sudo update-ca-trust
        echo "✓ 证书已成功安装到系统信任列表"
        
    else
        echo "警告：未识别的 Linux 发行版"
        echo "请手动将证书添加到系统信任列表"
        exit 1
    fi
    
    echo "注意：Linux 版本的 Desktop 已自动配置跳过证书验证"
    echo "此操作仅用于系统级证书信任"
    
else
    echo "错误：不支持的操作系统: $(uname)"
    exit 1
fi
