# dan-tick 品牌图标

这里的两张 PNG 是原始高清母版，保留透明背景。修改品牌视觉时先替换母版，再更新各平台的衍生资源。

| 母版 | 用途 | 生成资源 |
| --- | --- | --- |
| `app-icon-source.png` | 独立字母 D 徽标 | `build/appicon.png`、Windows ICO、Linux 图标、网页 favicon |
| `titlebar-wordmark-source.png` | 横向 dan-tick 字标 | `frontend/src/assets/branding/titlebar-wordmark.png` |

在项目根目录使用 ImageMagick 重新生成 PNG：

```sh
magick assets/branding/app-icon-source.png -resize 1024x1024 build/appicon.png
magick assets/branding/titlebar-wordmark-source.png -resize 450x150 frontend/src/assets/branding/titlebar-wordmark.png
magick build/appicon.png -resize 64x64 frontend/public/favicon.png

for size in 16 24 32 48 64 128 256; do
  output="build/linux/icons/hicolor/${size}x${size}/apps/dan-tick.png"
  mkdir -p "$(dirname "$output")"
  magick build/appicon.png -resize "${size}x${size}" "$output"
done
```

然后运行 `wails3 task common:generate:icons` 生成 Windows ICO（同时更新 macOS 模板图标）。Windows EXE 构建通过 `generate:syso` 嵌入 ICO；Linux AppImage 直接使用 `build/appicon.png`，DEB/RPM 等安装包使用对应分辨率的 hicolor 图标。

顶栏字标 PNG 为 450×150，实际显示为 105×35 CSS px。42 px 顶栏扣除 1 px 底边框后，可用高度为 41 px；35 px 字标居中时顶部位于 3 px，避免原先 28 px 字标居中后落在 6.5 px 的位置。字标主体同时增大，减少小尺寸下描边与高光拥挤；调整顶栏高度时需重新核对像素对齐。
