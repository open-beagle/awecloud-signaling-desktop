# Logo 使用说明

## Logo 位置

项目中的 Beagle logo 已应用到以下位置：

### Desktop 应用

- `build/appicon.png` - 主应用图标（源文件）
- `build/windows/icon.png` - Windows 图标
- `frontend/src/assets/logo.png` - 前端资源

### Web 应用

- `../web/public/logo.png` - Web 公共资源
- `../web/src/assets/logo.png` - Web 前端资源

## 更新 Logo

如果需要更新 logo，请按以下步骤操作：

### 1. 替换源文件

将新的 logo 图片保存为 `build/appicon.png`

### 2. 运行更新脚本

```bash
cd desktop
./scripts/update-icon.sh build/appicon.png
```

或手动复制：

```bash
# Desktop 前端
cp build/appicon.png frontend/src/assets/logo.png

# Windows 图标
cp build/appicon.png build/windows/icon.png

# Web 应用
cp build/appicon.png ../web/public/logo.png
cp build/appicon.png ../web/src/assets/logo.png
```

### 3. 重新构建

```bash
# Desktop 应用
wails build -platform windows/amd64

# Web 应用
cd ../web
npm run build
```

## Logo 规格建议

- **格式**: PNG（支持透明背景）
- **尺寸**: 512x512 或更大（正方形）
- **颜色**: 建议使用品牌色（当前为蓝白配色）
- **背景**: 透明或纯色

## 当前 Logo

当前使用的是 Beagle（比格犬）logo：

- 蓝白配色
- 简洁的图形设计
- 符合项目名称（open-beagle）

## 在界面中使用

### Desktop 登录页面

Logo 已自动显示在登录页面顶部：

```vue
<div class="logo-container">
  <img src="../assets/logo.png" alt="AWECloud Logo" class="logo" />
</div>
```

### Web 界面

可以在 Header 组件中添加 logo：

```vue
<img src="/logo.png" alt="AWECloud" class="header-logo" />
```

## 注意事项

1. **文件大小**: 保持 logo 文件大小合理（建议 < 500KB）
2. **版权**: 确保有权使用该 logo
3. **一致性**: 在所有平台保持 logo 一致
4. **可访问性**: 提供适当的 alt 文本

## 生成 Windows .ico 文件（可选）

如果需要生成 Windows .ico 格式：

```bash
# 安装 ImageMagick
sudo apt-get install imagemagick icoutils

# 生成 .ico 文件
convert build/appicon.png -resize 256x256 build/windows/icon-256.png
icotool -c -o build/windows/icon.ico build/windows/icon-256.png
```

然后在 `wails.json` 中配置：

```json
{
  "info": {
    "icon": "build/windows/icon.ico"
  }
}
```
