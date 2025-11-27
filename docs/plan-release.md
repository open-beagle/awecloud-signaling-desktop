# Desktop MVP Release Notes

**版本**: v1.0.0 (MVP)  
**发布日期**: 2025-11-27  
**构建状态**: ✅ 成功

## 📦 发布文件

### Windows 版本

**文件**: `build/bin/awecloud-desktop.exe`  
**大小**: 22MB  
**平台**: Windows 10/11 (64位)  
**架构**: amd64

## ✨ 功能特性

### 核心功能

- ✅ **用户认证**: 使用 Client ID 和 Secret 登录
- ✅ **服务列表**: 查看所有可访问的远程服务
- ✅ **服务连接**: 建立 STCP 隧道到远程服务
- ✅ **端口映射**: 在本地端口访问远程服务
- ✅ **连接管理**: 连接/断开服务，实时状态显示
- ✅ **配置持久化**: 自动保存服务器地址和用户信息

### 用户界面

- ✅ **现代化设计**: 基于 Element Plus 的美观界面
- ✅ **品牌标识**: 集成 Beagle logo
- ✅ **响应式布局**: 适配不同屏幕尺寸
- ✅ **状态反馈**: 清晰的连接状态和错误提示
- ✅ **中文界面**: 完整的中文本地化

## 🏗️ 技术架构

### 后端 (Go)

- **框架**: Wails v2.11.0
- **通信**: gRPC v1.77.0
- **隧道**: FRP v0.65.0
- **架构**: 单进程双线程
  - Desktop-Web 线程: gRPC 客户端
  - Desktop-FRP 线程: FRP 客户端

### 前端 (Vue 3)

- **框架**: Vue 3.2.37 + TypeScript 4.6.4
- **UI 库**: Element Plus 2.4.4
- **状态管理**: Pinia 2.1.7
- **路由**: Vue Router 4.2.5
- **构建工具**: Vite 3.0.7

## 📋 系统要求

### 最低要求

- **操作系统**: Windows 10 (64位) 或更高
- **内存**: 512MB 可用内存
- **磁盘**: 50MB 可用空间
- **网络**: 能够访问 AWECloud Server

### 依赖项

- **WebView2**: Windows 11 已预装，Windows 10 可能需要安装
  - 首次运行时会自动提示安装

## 🚀 安装和使用

### 安装

1. 下载 `awecloud-desktop.exe`
2. 双击运行（无需安装）
3. 如提示安装 WebView2，按提示操作

### 使用

1. **登录**
   - 输入 Server 地址（例如：`localhost:8080`）
   - 输入 Client ID 和 Secret
   - 点击"登录"

2. **连接服务**
   - 在服务列表中选择要连接的服务
   - 输入本地端口
   - 点击"连接"
   - 等待状态变为"已连接"

3. **访问服务**
   - 使用相应的客户端工具连接到 `localhost:本地端口`
   - 例如：`mysql -h 127.0.0.1 -P 3306 -u user -p`

详细使用说明见 [用户手册](docs/user-guide.md)

## 📁 配置文件

配置文件自动存储在：

**Windows**: `%APPDATA%\awecloud-desktop\config.json`

包含内容：
- 服务器地址
- Client ID（不包含密钥）

## 🔒 安全说明

- ✅ 所有通信通过 gRPC 和 WebSocket 加密
- ✅ STCP 隧道端到端加密
- ✅ 本地端口仅监听 127.0.0.1
- ⚠️ Client Secret 当前以明文存储（后续版本将改进）

## 🐛 已知限制

1. **平台支持**
   - 当前仅支持 Windows
   - macOS 和 Linux 支持将在后续版本添加

2. **FRP Visitor 管理**
   - 动态添加 Visitor 需要重启 FRP Service
   - 这是 FRP 库的限制

3. **配置管理**
   - 不支持多 Server 配置切换
   - 不支持配置导入/导出

4. **UI 功能**
   - 无系统托盘图标
   - 无开机自启动
   - 无自动重连

## 🔄 更新日志

### v1.0.0 (2025-11-27) - MVP 版本

**新增功能**:
- ✅ 完整的用户认证流程
- ✅ 服务列表查看和刷新
- ✅ STCP 隧道连接和断开
- ✅ 实时连接状态显示
- ✅ 配置持久化
- ✅ Beagle logo 集成
- ✅ 中文界面

**技术实现**:
- ✅ 单进程双线程架构
- ✅ gRPC 客户端通信
- ✅ FRP STCP Visitor 管理
- ✅ Go channel 进程内通信
- ✅ Vue 3 + TypeScript 前端
- ✅ Element Plus UI 组件

## 📚 文档

- [README](README.md) - 项目说明
- [开发指南](docs/development.md) - 开发和构建说明
- [用户手册](docs/user-guide.md) - 用户使用指南
- [Logo 说明](docs/logo.md) - Logo 使用和更新
- [开发进度](PROGRESS.md) - 详细的开发进度

## 🛠️ 开发信息

### 构建信息

- **构建工具**: Wails CLI v2.11.0
- **Go 版本**: 1.23
- **Node 版本**: 18+
- **构建时间**: ~15 秒
- **构建命令**: `wails build -platform windows/amd64`

### 代码统计

- **Go 代码**: ~1500 行
- **TypeScript/Vue**: ~800 行
- **总文件数**: ~30 个
- **依赖包**: ~150 个

## 🤝 贡献

本项目是 AWECloud Signaling 系统的一部分。

- **主项目**: https://github.com/open-beagle/awecloud-signaling-server
- **Desktop 仓库**: https://github.com/open-beagle/awecloud-signaling-desktop

## 📄 许可证

与主项目相同

## 🔮 后续版本计划

### v1.1 (计划中)

- [ ] macOS 支持
- [ ] Linux 支持
- [ ] 自动重连机制
- [ ] 配置导入/导出
- [ ] 详细日志查看

### v1.2 (计划中)

- [ ] 系统托盘集成
- [ ] 开机自启动
- [ ] 多 Server 配置
- [ ] 自动更新功能

### v2.0 (计划中)

- [ ] 服务自动发现
- [ ] 连接统计和监控
- [ ] 高级配置选项
- [ ] 插件系统

## 📞 支持

如遇问题，请：

1. 查看 [用户手册](docs/user-guide.md)
2. 查看 [开发文档](docs/development.md)
3. 联系系统管理员
4. 提交 Issue 到 GitHub

---

**感谢使用 AWECloud Desktop！** 🎉
