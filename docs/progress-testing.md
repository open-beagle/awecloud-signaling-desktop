# Desktop MVP 测试指南

**测试版本**: v1.0.0 (MVP)  
**测试日期**: 2025-11-27  
**测试平台**: Windows 10/11

## 测试前准备

### 1. 环境要求

- ✅ Windows 10/11 (64位)
- ✅ WebView2 运行时（Windows 11 已预装）
- ✅ Server 和 Agent 正在运行
- ✅ 已创建 Client 账号和 STCP 实例

### 2. 启动 Server 和 Agent

**终端 1 - Server**:
```bash
cd /path/to/awecloud-signaling-server
./scripts/run_server.sh
```

**终端 2 - Agent**:
```bash
cd /path/to/awecloud-signaling-server
./scripts/run_agent.sh
```

### 3. 创建测试数据

#### 3.1 登录管理后台

```bash
# 获取管理员 Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.token')

echo "Token: $TOKEN"
```

#### 3.2 创建 Client 账号

```bash
# 创建 Client
CLIENT_RESPONSE=$(curl -s -X POST http://localhost:8080/api/clients \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"client_id":"test@example.com"}')

echo "$CLIENT_RESPONSE" | jq .

# 记录 client_secret（重要！）
CLIENT_SECRET=$(echo "$CLIENT_RESPONSE" | jq -r '.client_secret')
echo "Client Secret: $CLIENT_SECRET"
```

**重要**: 请保存 `CLIENT_SECRET`，后续登录需要使用。

#### 3.3 创建 STCP 实例

```bash
# 获取 Agent ID
AGENT_ID=$(curl -s http://localhost:8080/api/agents \
  -H "Authorization: Bearer $TOKEN" | jq -r '.agents[0].id')

echo "Agent ID: $AGENT_ID"

# 创建 STCP 实例
INSTANCE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/stcp-instances \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"agent_id\": $AGENT_ID,
    \"instance_name\": \"test-service\",
    \"local_ip\": \"127.0.0.1\",
    \"local_port\": 8080,
    \"description\": \"测试服务（Server HTTP端口）\"
  }")

echo "$INSTANCE_RESPONSE" | jq .

# 记录 instance_id
INSTANCE_ID=$(echo "$INSTANCE_RESPONSE" | jq -r '.instance.id')
echo "Instance ID: $INSTANCE_ID"
```

#### 3.4 授权 Client 访问

```bash
# 获取 Client ID
CLIENT_ID=$(curl -s http://localhost:8080/api/clients \
  -H "Authorization: Bearer $TOKEN" | jq -r '.clients[0].id')

echo "Client ID: $CLIENT_ID"

# 授权访问
curl -X POST http://localhost:8080/api/stcp-instances/$INSTANCE_ID/grant-access \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"client_id\": $CLIENT_ID}"
```

### 4. 测试凭证

请记录以下信息用于 Desktop 登录：

```
服务器地址: localhost:8080
Client ID: test@example.com
Client Secret: [上面创建时返回的 secret]
```

## 测试步骤

### 测试 1: 应用启动

**步骤**:
1. 双击 `desktop/build/bin/awecloud-desktop.exe`
2. 等待应用启动

**预期结果**:
- ✅ 应用窗口打开
- ✅ 显示登录界面
- ✅ 界面显示 Beagle logo
- ✅ 有服务器地址、Client ID、Client Secret 输入框

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 2: 用户登录

**步骤**:
1. 输入服务器地址：`localhost:8080`
2. 输入 Client ID：`test@example.com`
3. 输入 Client Secret：（你记录的 secret）
4. 点击"登录"按钮

**预期结果**:
- ✅ 登录成功
- ✅ 跳转到服务列表页面
- ✅ 顶部显示用户信息

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 3: 查看服务列表

**步骤**:
1. 登录成功后自动显示服务列表

**预期结果**:
- ✅ 显示至少一个服务（test-service）
- ✅ 服务卡片显示：
  - 服务名称：test-service
  - Agent 名称
  - 远程端口：8080
  - 描述：测试服务
  - 状态：未连接

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 4: 刷新服务列表

**步骤**:
1. 点击右上角"刷新"按钮

**预期结果**:
- ✅ 显示"刷新成功"提示
- ✅ 服务列表更新

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 5: 连接服务

**步骤**:
1. 在服务卡片中输入本地端口：`18080`
2. 点击"连接"按钮
3. 等待连接建立

**预期结果**:
- ✅ 状态变为"连接中"
- ✅ 几秒后状态变为"已连接"
- ✅ 显示本地端口：18080
- ✅ 连接按钮变为"断开连接"按钮

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 6: 访问服务

**步骤**:
1. 打开浏览器
2. 访问 `http://localhost:18080/health`

**预期结果**:
- ✅ 返回 JSON：`{"status":"ok"}`
- ✅ 说明隧道已建立，可以访问远程服务

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 7: 断开连接

**步骤**:
1. 点击"断开连接"按钮

**预期结果**:
- ✅ 显示"已断开连接"提示
- ✅ 状态变为"未连接"
- ✅ 按钮变回"连接"按钮
- ✅ 浏览器访问 `http://localhost:18080` 失败（连接被拒绝）

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 8: 重新连接

**步骤**:
1. 再次输入本地端口：`18080`
2. 点击"连接"按钮

**预期结果**:
- ✅ 能够重新连接成功
- ✅ 状态变为"已连接"

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 9: 退出登录

**步骤**:
1. 点击右上角"退出登录"按钮

**预期结果**:
- ✅ 所有连接自动断开
- ✅ 返回登录页面
- ✅ 显示"已退出登录"提示

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

### 测试 10: 配置持久化

**步骤**:
1. 登录后退出应用
2. 重新打开应用

**预期结果**:
- ✅ 服务器地址自动填充（上次使用的地址）
- ✅ Client ID 自动填充（上次使用的 ID）
- ✅ Client Secret 需要重新输入（安全考虑）

**实际结果**:
- [ ] 通过
- [ ] 失败（原因：_____________）

---

## 测试总结

### 测试统计

- 总测试项：10
- 通过：___
- 失败：___
- 通过率：___%

### 发现的问题

1. 
2. 
3. 

### 性能观察

- 应用启动时间：___ 秒
- 登录响应时间：___ 秒
- 连接建立时间：___ 秒
- 内存使用：___ MB
- CPU 使用：___%

### 用户体验

- [ ] 界面美观
- [ ] 操作流畅
- [ ] 提示清晰
- [ ] 错误处理友好

### 建议改进

1. 
2. 
3. 

## 高级测试（可选）

### 测试 11: 多服务连接

**步骤**:
1. 创建多个 STCP 实例
2. 同时连接多个服务

**预期结果**:
- ✅ 能够同时连接多个服务
- ✅ 每个服务独立管理

### 测试 12: 端口冲突

**步骤**:
1. 使用已被占用的本地端口

**预期结果**:
- ✅ 显示错误提示："端口已被占用"

### 测试 13: 网络中断

**步骤**:
1. 连接服务后
2. 断开网络
3. 恢复网络

**预期结果**:
- ✅ 显示连接错误
- ✅ 网络恢复后可以重新连接

### 测试 14: 长时间运行

**步骤**:
1. 保持连接 30 分钟以上

**预期结果**:
- ✅ 连接保持稳定
- ✅ 无内存泄漏
- ✅ 性能稳定

## 测试环境信息

- **操作系统**: Windows ___ (版本：___)
- **WebView2 版本**: ___
- **Server 版本**: ___
- **Agent 版本**: ___
- **Desktop 版本**: v1.0.0 (MVP)
- **测试日期**: ___
- **测试人员**: ___

## 附录

### 查看日志

**Desktop 日志**:
- 位置：`%APPDATA%\awecloud-desktop\logs\` (如果有)
- 或查看控制台输出

**Server 日志**:
- 查看 Server 终端输出

**Agent 日志**:
- 查看 Agent 终端输出

### 清理测试数据

```bash
# 删除数据库（重新开始）
rm -f data/server.db

# 重启 Server
./scripts/run_server.sh
```

### 常见问题

**Q: Desktop 无法启动？**  
A: 检查是否安装了 WebView2 运行时

**Q: 登录失败？**  
A: 检查 Server 是否运行，Client Secret 是否正确

**Q: 看不到服务列表？**  
A: 检查是否已授权 Client 访问 STCP 实例

**Q: 连接失败？**  
A: 检查 Agent 是否在线，FRP 服务是否正常

---

**测试完成后，请将此文档连同测试结果一起提交。**
