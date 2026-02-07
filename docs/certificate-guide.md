# 证书安装指南

## 为什么需要安装证书？

AWECloud Signaling Desktop 使用 HTTPS 加密连接到服务器。如果服务器使用自签名证书（常见于内网环境或开发测试环境），您需要将证书添加到系统的受信任列表中。

## 获取证书文件

请联系您的系统管理员获取服务器证书文件（通常是 `.crt` 或 `.pem` 文件）。

## 安装步骤

### Windows

#### 方法一：图形界面（推荐）

1. 右键点击证书文件，选择"安装证书"
2. 选择"本地计算机"（需要管理员权限）
3. 点击"下一步"
4. 选择"将所有的证书都放入下列存储"
5. 点击"浏览"，选择"受信任的根证书颁发机构"
6. 点击"确定"，然后"下一步"
7. 点击"完成"

#### 方法二：命令行

以管理员身份打开 PowerShell，运行：

```powershell
# 使用提供的脚本
.\desktop\scripts\install-cert.ps1 -CertFile "C:\path\to\server.crt"

# 或直接使用 certutil
certutil -addstore -f "ROOT" "C:\path\to\server.crt"
```

### macOS

#### 方法一：图形界面（推荐）

1. 双击证书文件
2. 在弹出的"钥匙串访问"窗口中，将证书拖到"系统"钥匙串
3. 输入管理员密码
4. 在钥匙串访问中找到刚添加的证书，双击打开
5. 展开"信任"部分
6. 将"使用此证书时"设置为"始终信任"
7. 关闭窗口，再次输入管理员密码确认

#### 方法二：命令行

打开终端，运行：

```bash
# 使用提供的脚本
bash desktop/scripts/install-cert.sh /path/to/server.crt

# 或直接使用 security 命令
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /path/to/server.crt
```

### Linux

**好消息**：Linux 版本的 Desktop 已自动配置跳过证书验证，无需手动操作！

如果您仍然希望将证书添加到系统信任列表（可选），可以运行：

```bash
# 使用提供的脚本
bash desktop/scripts/install-cert.sh /path/to/server.crt
```

脚本会自动检测您的 Linux 发行版（Ubuntu/Debian 或 CentOS/RHEL）并执行相应的安装命令。

## 验证安装

安装证书后，请按以下步骤验证：

1. 重启 Desktop 应用
2. 输入服务器地址（例如：`https://signal.example.com`）
3. 点击"登录"
4. 如果能正常打开登录页面，说明证书已成功安装

## 常见问题

### Q: 安装证书后仍然无法连接？

A: 请尝试以下步骤：

1. 确认证书文件是正确的服务器证书
2. 重启 Desktop 应用
3. 在 Windows 上，确认证书已添加到"受信任的根证书颁发机构"
4. 在 macOS 上，确认证书的信任设置为"始终信任"

### Q: 如何删除已安装的证书？

**Windows**:

1. 按 `Win + R`，输入 `certmgr.msc`
2. 展开"受信任的根证书颁发机构" > "证书"
3. 找到对应的证书，右键删除

**macOS**:

1. 打开"钥匙串访问"
2. 在"系统"钥匙串中找到证书
3. 右键删除

**Linux**:

```bash
# Ubuntu/Debian
sudo rm /usr/local/share/ca-certificates/server.crt
sudo update-ca-certificates --fresh

# CentOS/RHEL
sudo rm /etc/pki/ca-trust/source/anchors/server.crt
sudo update-ca-trust
```

### Q: 为什么 Linux 不需要安装证书？

A: Linux 版本的 Desktop 在启动时会自动设置环境变量 `WEBKIT_IGNORE_TLS_ERRORS=1`，这会让 WebView 跳过证书验证。这是为了简化 Linux 用户的使用体验。

### Q: 这样做安全吗？

A:

- **开发/测试环境**：使用自签名证书是常见做法，安装证书是安全的
- **生产环境**：建议使用有效的 SSL 证书（如 Let's Encrypt 免费证书）
- **内网环境**：建议使用企业内部 CA 签发的证书

## 技术支持

如果您在安装证书过程中遇到问题，请联系您的系统管理员或技术支持团队。
