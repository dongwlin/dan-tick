# Linux 运行时与打包环境

本目录是 Linux 自包含 AppImage 的**唯一构建环境**。`env.sh` 固定全部输入，`Dockerfile` 提供两个阶段：编译打过重定位补丁的 WebKitGTK（`webkit-build` / `webkit-runtime`），以及编译应用并打包 AppImage 的固定镜像（`packaging`）。

## 为什么需要自己编译 WebKit

发行版的 WebKitGTK 在构建时把安装前缀写进二进制：helper 目录（`PKGLIBEXECDIR`，Ubuntu 为 `/usr/lib/x86_64-linux-gnu/webkitgtk-6.0`）、注入库目录（`PKGLIBDIR/injected-bundle`）以及沙箱里要绑定的目录。AppImage 挂载点每次运行都不同，发行版 release 构建又会把读取 `WEBKIT_EXEC_PATH` 的分支编译掉（只在 `ENABLE(DEVELOPER_MODE)` 下存在），所以把这些库放进 AppImage 后，WebKit 会去宿主的绝对路径找 helper，找不到就崩溃——这正是旧包在 Fedora 上启动失败的原因。

`relocatable-runtime.patch`（相对 WebKitGTK 源码根，`-p1`）用**实际加载到的库位置**替换这些编译期路径，共 5 个文件、约 200 行新增：

| 文件 | 作用 |
| --- | --- |
| `Source/WebKit/Shared/glib/WebKitRuntimePaths.{h,cpp}` | 新增：用 `dladdr` 从加载到的 `libwebkitgtk-6.0.so` 推导运行目录 `<libdir>/webkitgtk-6.0`、库目录 `<libdir>`、数据目录 `<prefix>/share`；沙箱工具在系统路径存在时优先使用系统版本，否则使用运行目录内的副本 |
| `Source/WebKit/Shared/glib/ProcessExecutablePathGLib.cpp` | helper 进程查找顺序：`WEBKIT_EXEC_PATH`（开发者模式）→ 推导出的运行目录 → 编译期 `PKGLIBEXECDIR` |
| `Source/WebKit/UIProcess/API/glib/WebKitWebContext.cpp` | 注入库目录改为推导出的运行目录，并回落到 `PKGLIBDIR/injected-bundle` |
| `Source/WebKit/UIProcess/Launcher/glib/BubblewrapLauncher.cpp` | bwrap 与 xdg-dbus-proxy 使用统一的解析函数；沙箱内先 `--dir` 补齐父目录，再把库目录、运行目录与数据目录 `--ro-bind-try` 回原路径（bwrap 的绑定源相对原始根解析，因此被 `--tmpfs /tmp` 遮住的 AppImage 挂载点仍可绑定） |
| `Source/WebKit/UIProcess/Launcher/glib/XDGDBusProxy.cpp` | 与上面使用同一个沙箱工具解析函数，保证启动路径与沙箱内绑定路径一致 |

补丁只在“推导出的目录真实存在”时生效，因此系统安装的 WebKit 行为不变。它不关闭沙箱、不设置 setuid、不做无差别的 `/usr` 字符串替换。

安装布局保持发行版约定（`CMAKE_INSTALL_LIBDIR=lib/x86_64-linux-gnu`、helper 与注入库都在 `<libdir>/webkitgtk-6.0`），这样 AppImage 内的相对结构与构建环境一致，推导逻辑不需要额外分支。

## 构建选项

`Dockerfile` 中的选项是刻意选择的：

- `USE_GTK4=ON`、`ENABLE_BUBBLEWRAP_SANDBOX=ON`：保持项目当前技术栈与沙箱。
- `ENABLE_VIDEO=OFF`、`ENABLE_WEB_AUDIO=OFF`、`ENABLE_GAMEPAD=OFF`、`USE_FLITE=OFF`：dan-tick 渲染自己的 Vue 界面，不播放媒体，去掉 GStreamer、libmanette、Flite 依赖可以显著缩小包体与运行时依赖面。如果以后需要媒体播放，需要重新评估并重建运行时。
- `ENABLE_JOURNALD_LOG=OFF`：避免把 libsystemd 带进包内。
- `ENABLE_DOCUMENTATION=OFF`、`ENABLE_INTROSPECTION=OFF`、`ENABLE_MINIBROWSER=OFF`：只保留运行所需内容。

## 重建、发布与固定

```sh
# 重建并导出运行时（需要 Docker；数十分钟）
wails3 task linux:create:webkit-runtime

# 或在 GitHub Actions 中触发 “Linux WebKit Runtime” workflow
```

脚本会把结果打包为 `bin/runtime/dan-tick-webkit-runtime-<runtime-id>.tar.gz`，内部带 `runtime-info.txt`，并打印 SHA-256。发布与固定步骤：

```sh
gh release create "$RUNTIME_RELEASE_TAG" --title "$RUNTIME_ID" \
  --notes 'Pinned WebKitGTK runtime for dan-tick AppImages' \
  bin/runtime/dan-tick-webkit-runtime-*.tar.gz
# 把打印出的 sha256 写入 build/linux/runtime/env.sh 的 RUNTIME_ASSET_SHA256
```

`RUNTIME_RELEASE_TAG`、`RUNTIME_ASSET` 等名称由 `env.sh` 推导；改动 WebKit 版本、补丁或构建选项时必须同时提升 `RUNTIME_REVISION`，否则同一 tag 会对应不同内容。AppImage 构建会校验 SHA-256，校验失败不会继续打包；Artifacts 会保留运行时产物 30 天，release 资产用于长期复用。CI 的常规流程只下载并用缓存产物，不重新编译 WebKit。

## 打包镜像

`packaging` 阶段基于同一个 Ubuntu 24.04（glibc 2.39）基线，安装 GTK4 / WebKitGTK 6.0 运行库与开发包、GIO TLS 模块、gdk-pixbuf 加载器、GLib schemas、Adwaita 图标主题、`bubblewrap`、`xdg-dbus-proxy`、`patchelf`，并从固定 digest 的官方镜像复制 Go 与 Node 工具链。应用在这个镜像里编译，因此二进制、包内 GTK / WebKit、GIO 模块、图片加载器与 schemas 都来自同一次环境；`build/linux/appimage/build.sh` 只从这个环境收集依赖闭包。

不随包分发的内容及其原因见 [构建与发布](../../../docs/构建与发布.md#linux-appimage-自带运行时与兼容范围)：glibc / libstdc++ / libgcc 必须与宿主的 NSS 与图形驱动一致，字体与 GPU 驱动属于宿主。
