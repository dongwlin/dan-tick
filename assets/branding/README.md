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

## 托盘图标

`tray-icon.svg` 是针对小尺寸绘制的简化母版，保留 D 轮廓与时钟指针，去掉高清应用图标的高光、渐变与装饰。它只用于系统托盘，不替换应用图标或顶栏字标。

| 平台 | 运行时资源 | 显示方式 |
| --- | --- | --- |
| Windows 浅色系统主题 | `internal/desktop/icons/tray-light.ico` | 深色 D，包含 16 / 32 px |
| Windows 深色系统主题 | `internal/desktop/icons/tray-dark.ico` | 白色 D，包含 16 / 32 px |
| Linux | `internal/desktop/icons/tray-linux.png` | 24 px 蓝色 D，白色外缘和深色描边适配深浅面板 |

在项目根目录重新生成托盘资源（需要 ImageMagick）：

```sh
sh assets/branding/generate-tray-icons.sh
```

Go 通过 `internal/desktop/tray.go` 嵌入这些资源，构建不需要 ImageMagick。Windows 使用 Wails 的系统主题图标切换；Linux 的 Wails `v3.0.0-beta.25` 没有可靠的托盘面板主题切换能力，因此使用同一张双色描边图标，不调用 `SetDarkModeIcon` 覆盖它。前端 UI 主题与系统托盘主题各自独立。

托盘菜单提供显示、查看运行状态、暂停 / 恢复自动调度和退出应用。暂停只控制自动调度，退出复用现有的任务警告与业务收尾。Windows 单击托盘图标即恢复主窗口；Linux 双击图标恢复主窗口，单击和右键只打开菜单。Wails `v3.0.0-beta.25` 把 StatusNotifierItem 的 `Activate`（双击）与 dbusmenu 的菜单打开通知接到同一个点击回调，[`internal/desktop/tray.go`](../../internal/desktop/tray.go) 因此按当前调用来源归因，只对 `Activate` 恢复窗口；该归因依赖 Wails 内部的 Linux 托盘方法名，升级 Wails 后必须重新实测两种手势（归因失效时双击不恢复窗口，不会改回“弹菜单也拉起窗口”）。在 Linux 没有 StatusNotifier 托盘宿主时，仍可通过再次启动应用恢复主窗口；托盘不会成为唯一恢复入口。

### 目标平台人工验收（待完成）

- Windows：深浅系统主题及运行中切换、100% / 200% DPI、Explorer 重启后图标恢复、点击图标与菜单恢复最小化 / 隐藏的主窗口。
- Linux：KDE 的 StatusNotifier、GNOME 的托盘扩展、深浅面板，以及没有托盘支持的桌面；核对图标、菜单、双击图标恢复主窗口（单击和右键只弹菜单）和再次启动的恢复入口。
- 两平台：菜单状态与设置页的 Go 权威状态一致，暂停 / 恢复不改任务定义，操作失败有可见反馈；隐藏时调度继续运行，退出警告可见、取消后继续运行、确认后完整收尾。使用测试账号替身或隔离数据，不触发真实 B 站发送。

资源渲染、自动测试和 Linux 构建不能替代以上桌面验收。
