# Desktop 开发指南

## 项目结构

```
desktop/
├── internal/              # Go 后端代码
│   ├── client/           # Desktop-Web 线程（gRPC 客户端）
│   ├── frp/              # Desktop-FRP 线程（FRP 客户端）
│   ├── config/           # 配置管理
│   └── models/           # 数据模型
├── frontend/             # Vue 3 前端代码
│   ├── src/
│   │   ├── views/       # 页面组件
│   │   ├── components/  # 通用组件
│   │   ├── stores/      # Pinia 状态管理
│   │   └── router/      # Vue Router 配置
│   └── wailsjs/         # Wails 自动生成的绑定
├── app.go                # Wails 应用主结构
└── main.go               # 应用入口
```

## 开发环境设置

### 前置要求

- Go 1.23+
- Node.js 18+
- Wails CLI v2.11.0+

### 安装 Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 安装依赖

```bash
# 安装 Go 依赖
go mod download

# 安装前端依赖
cd frontend
npm install
cd ..
```

## 开发流程

### 1. 启动开发模式

```bash
# 方式 1: 使用 Wails CLI
wails dev

# 方式 2: 使用脚本
./scripts/dev.sh
```

开发模式特性：
- 前端热重载
- Go 代码自动重新编译
- 实时调试

### 2. 修改代码

#### 后端开发

修改 Go 代码后，Wails 会自动重新编译。

**暴露新方法给前端**：

1. 在 `app.go` 中添加方法：
```go
func (a *App) MyNewMethod(param string) (string, error) {
    // 实现逻辑
    return "result", nil
}
```

2. 重新生成绑定：
```bash
wails generate module
```

3. 在前端使用：
```typescript
import { MyNewMethod } from '../../wailsjs/go/main/App'

const result = await MyNewMethod("param")
```

#### 前端开发

修改 Vue 组件后会自动热重载。

**添加新页面**：

1. 在 `frontend/src/views/` 创建组件
2. 在 `frontend/src/router/index.ts` 添加路由
3. 在需要的地方使用 `router.push()` 导航

**添加新状态**：

1. 在 `frontend/src/stores/` 创建 store
2. 使用 Pinia 的 `defineStore`
3. 在组件中使用 `useXxxStore()`

### 3. 构建应用

```bash
# 构建当前平台
wails build

# 构建指定平台
wails build -platform windows/amd64

# 使用脚本
./scripts/build.sh
```

输出位置：`build/bin/awecloud-desktop`

## 架构说明

### 单进程双线程架构

Desktop 应用是一个单一进程，包含两个工作线程：

1. **Desktop-Web 线程** (`internal/client/`)
   - 通过 gRPC 连接 Server
   - 处理认证和服务查询
   - 与 Desktop-FRP 线程通信

2. **Desktop-FRP 线程** (`internal/frp/`)
   - 通过 WebSocket 连接 Server-FRP
   - 管理 STCP Visitor
   - 建立本地端口映射

### 进程内通信

两个线程通过 Go channel 通信：

```go
// 命令通道：Desktop-Web → Desktop-FRP
commandChan chan *models.VisitorCommand

// 状态通道：Desktop-FRP → Desktop-Web
statusChan chan *models.VisitorStatus
```

### 前后端通信

前端通过 Wails 绑定调用 Go 方法：

```typescript
// 前端调用
import { Login } from '../../wailsjs/go/main/App'
await Login(serverAddr, clientId, clientSecret)
```

```go
// 后端实现
func (a *App) Login(serverAddr, clientID, clientSecret string) error {
    // 实现逻辑
}
```

## 调试技巧

### 1. 查看日志

开发模式下，日志会输出到终端：

```go
log.Printf("[Desktop-Web] Message: %s", msg)
```

### 2. 前端调试

在开发模式下，可以使用浏览器开发者工具：
- 右键 → 检查元素
- 或按 F12

### 3. Go 调试

使用 Delve 调试器：

```bash
# 安装 Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 调试
dlv debug
```

## 常见问题

### 1. 前端依赖安装失败

```bash
# 清理缓存
cd frontend
rm -rf node_modules package-lock.json
npm install
```

### 2. Wails 绑定未更新

```bash
# 重新生成绑定
wails generate module
```

### 3. 构建失败

```bash
# 清理构建缓存
wails build -clean
```

### 4. FRP 连接失败

检查：
- Server 地址是否正确
- Server-FRP 是否运行在端口 7000
- 网络连接是否正常

## 测试

### 单元测试

```bash
# 运行 Go 测试
go test ./...

# 运行前端测试（需要配置）
cd frontend
npm test
```

### 集成测试

1. 启动 Server
2. 启动 Agent
3. 启动 Desktop 应用
4. 测试完整流程

## 发布流程

### 1. 更新版本号

在 `wails.json` 中更新版本号。

### 2. 构建发布版本

```bash
# Windows
wails build -platform windows/amd64 -clean

# 输出: build/bin/awecloud-desktop.exe
```

### 3. 打包（可选）

使用 NSIS 或 Inno Setup 创建安装程序。

## 参考资料

- [Wails 文档](https://wails.io/docs/introduction)
- [Vue 3 文档](https://vuejs.org/)
- [Element Plus 文档](https://element-plus.org/)
- [Pinia 文档](https://pinia.vuejs.org/)
- [主项目设计文档](../../docs/design_desktop.md)
