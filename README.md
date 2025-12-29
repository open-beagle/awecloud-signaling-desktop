# AWECloud Signaling Desktop

Desktop 客户端应用 - 基于 Wails 的跨平台桌面应用

## 项目信息

- **技术栈**: Wails v2 + Go + Vue 3 + TypeScript
- **主项目**: https://github.com/open-beagle/awecloud-signaling-server
- **设计文档**: 见主项目 `docs/design_desktop.md`

## 快速启动

### 前置要求

- Go 1.23+
- Node.js 18+
- Wails CLI v2.11.0+

安装 Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 开发模式

```bash
# 安装前端依赖
cd frontend && npm install && cd ..

# 启动开发模式（热重载）
wails dev

# 或使用脚本
./scripts/dev.sh
```

### 构建

```bash
# 构建当前平台
wails build

# 或使用脚本
./scripts/build.sh
```

输出位置: `build/bin/awecloud-desktop`

## 文档规范

### 3.1 根目录文档规范

根目录下不允许 README.md 之外的文档。

### 3.2 文档基地

所有文档统一存放在 `docs/` 目录下。

### 3.3 文档命名规范

docs 下所有文档使用小写命名。

### 3.4 文档分类规范

- **设计类**: `design-` 开头（如 `design-logo.md`）
- **开发进度类**: `progress-` 开头（如 `progress-mvp.md`）
- **计划类**: `plan-` 开头（如 `plan-roadmap.md`）
- **开发进展**: `development-` 开头（如 `development-guide.md`）

## 文档结构

```
docs/
├── development-guide.md    # 开发指南
├── user-guide.md          # 用户手册
├── design-logo.md         # Logo 设计说明
├── progress-mvp.md        # MVP 开发进度
├── progress-testing.md    # 测试指南
├── progress-results.md    # 测试结果
└── plan-release.md        # 发布计划
```

详见 [docs/README.md](docs/README.md)

## 许可证

与主项目相同
