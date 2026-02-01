# Desktop 快速启动指南

## 前置条件

Desktop 客户端需要连接到 Server 才能正常工作。

### 1. 启动 Server

在另一个终端中启动 Server：

```bash
# 从项目根目录
cd C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server

# 启动 Server
bash scripts/run_server.sh

# 或者直接运行
go run cmd/server/main.go
```

Server 默认监听：

- HTTP API: `http://localhost:8080`
- gRPC: `localhost:8081`

### 2. 配置 Desktop

Desktop 会自动查找配置文件：

**配置文件位置：**

- Windows: `%APPDATA%\awecloud-signaling-desktop\config.json`
- 或当前目录: `config.json`

**首次运行：**

Desktop 会使用默认配置：

- Server 地址: `localhost:8081`
- 需要在界面中输入 Client ID 和 Secret

**手动创建配置（可选）：**

创建 `config.json`：

```json
{
  "server_address": "localhost:8081",
  "client_id": "admin@example.com",
  "client_secret": "your-password"
}
```

### 3. 启动 Desktop

```powershell
# 开发模式（推荐，需要管理员权限）
Start-Process powershell -Verb RunAs -ArgumentList "-ExecutionPolicy Bypass -File desktop\scripts\dev.ps1 -BuildVersion v0.2.3"

# 或运行已构建的程序（需要管理员权限）
Start-Process desktop\build\bin\awecloud-signaling-desktop.exe -Verb RunAs
```

**说明：** `Start-Process -Verb RunAs` 类似 Linux 的 `sudo`，显式要求管理员权限。

## 常见错误

### 错误 1：连接失败

```
rpc error: code = Unavailable desc = connection error
```

**原因：** Server 未启动或地址配置错误

**解决：**

1. 确认 Server 正在运行
2. 检查 Server 地址配置
3. 检查防火墙设置

### 错误 2：TLS 握手失败

```
transport: authentication handshake failed: remote error: tls: no application protocol
```

**原因：** gRPC ALPN 协议协商失败

**可能的原因：**

1. Server 的 gRPC 配置问题
2. TLS 证书配置问题
3. 客户端和服务端协议不匹配

**解决：**

1. 检查 Server 的 gRPC 配置
2. 确认是否启用了 TLS
3. 如果是开发环境，可能需要禁用 TLS

### 错误 3：认证失败

```
authentication failed: authentication failed
```

**原因：** Client ID 或 Secret 错误

**解决：**

1. 在 Server Web 界面创建 Client
2. 使用正确的 Client ID 和 Secret
3. 检查 Server 的用户管理

## 开发环境快速启动

### 终端 1：启动 Server

```bash
cd C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server
go run cmd/server/main.go
```

### 终端 2：启动 Desktop（需要管理员权限）

```powershell
cd C:\Users\Mengk\go\src\github.com\open-beagle\awecloud-signaling-server

# 使用管理员权限启动（类似 Linux sudo）
Start-Process powershell -Verb RunAs -ArgumentList "-ExecutionPolicy Bypass -File desktop\scripts\dev.ps1 -BuildVersion v0.2.3"
```

### 终端 3：访问 Web 管理界面（可选）

浏览器打开：http://localhost:8080

创建 Client 用户，获取 ID 和 Secret。

## 配置说明

### Server 地址格式

```
localhost:8081          # 本地开发
192.168.1.100:8081     # 局域网
example.com:8081       # 域名
```

### Client 认证

Desktop 使用 Client ID 和 Secret 进行认证：

1. **首次登录：** 在界面输入 Client ID 和 Secret
2. **记住密码：** 勾选"记住我"，下次自动登录
3. **设备令牌：** 登录成功后会生成设备令牌，绑定硬件指纹

### 配置文件结构

```json
{
  "server_address": "localhost:8081", // Server gRPC 地址
  "client_id": "user@example.com", // Client ID
  "client_secret": "password123" // Client Secret（加密存储）
}
```

## 调试技巧

### 1. 查看详细日志

Desktop 会输出详细的日志信息，包括：

- 连接状态
- 认证过程
- gRPC 调用
- 错误信息

### 2. 检查 Server 状态

```bash
# 检查 Server 是否运行
netstat -ano | findstr :8081

# 测试 gRPC 连接（需要 grpcurl）
grpcurl -plaintext localhost:8081 list
```

### 3. 使用 Wireshark

抓包分析 gRPC 通信，查看具体的错误信息。

## 生产环境部署

### 1. 构建 Desktop

```powershell
# 使用管理员权限构建
Start-Process powershell -Verb RunAs -ArgumentList "-ExecutionPolicy Bypass -File desktop\scripts\build.ps1 -BuildVersion v1.0.0 -BuildAddress server.example.com:8081"
```

### 2. 分发程序

将 `desktop\build\bin\awecloud-signaling-desktop.exe` 分发给用户。

### 3. 用户配置

用户首次运行时：

1. 输入 Server 地址（如果未编译到程序中）
2. 输入 Client ID 和 Secret
3. 勾选"记住我"

## 故障排除

详见 [troubleshooting.md](troubleshooting.md)

## 参考文档

- [开发指南](development-guide.md)
- [构建指南](build-windows.md)
- [故障排除](troubleshooting.md)
