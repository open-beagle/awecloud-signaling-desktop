# AWECloud Desktop 文档

## 文档索引

### 开发文档
- [development-guide.md](development-guide.md) - 开发指南

### 用户文档
- [user-guide.md](user-guide.md) - 用户手册

### 设计文档
- [design-logo.md](design-logo.md) - Logo 设计说明

### 进度文档
- [progress-mvp.md](progress-mvp.md) - MVP 开发进度
- [progress-testing.md](progress-testing.md) - 测试指南
- [progress-results.md](progress-results.md) - 测试结果

### 计划文档
- [plan-release.md](plan-release.md) - 发布计划

## 快速开始

### 开发模式

```bash
# 安装前端依赖
cd frontend && npm install && cd ..

# 启动开发模式（热重载）
wails dev
```

### 构建

```bash
# 构建当前平台
wails build
```

### 配置

配置文件位置：
- **Windows**: `%APPDATA%\awecloud-desktop\config.json`
- **macOS**: `~/Library/Application Support/awecloud-desktop/config.json`
- **Linux**: `~/.config/awecloud-desktop/config.json`

配置示例见 `config/config.example.json`
