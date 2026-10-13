# dan-tick 构建与打包工作约定

本文件仅适用于 `build/` 及其子目录，补充 [根目录指令](../AGENTS.md)。根 `Taskfile.yml`、`go.mod`、`.github/workflows/` 及前端配置不在本文件作用域内；修改这些文件时遵守其对应指令，并按需核对共享构建链。

工具链与产品支持范围遵守根文档及 [技术栈选型](../docs/技术栈选型.md)。这里仍有 Wails 模板配置；以实际 Taskfile 依赖和 PRD 为准，不把模板任务的存在视为产品已支持对应平台或部署模式。

## 1. 文件职责与构建链

| 路径（相对本目录） | 职责 |
| --- | --- |
| `Taskfile.yml` | 共用依赖安装、前端脚本、Binding / 图标生成、模块整理及模板辅助任务 |
| `config.yml` | Wails 开发模式监听和执行配置，以及构建资源生成所用的 `info` / 文件关联元数据 |
| `windows/Taskfile.yml`、`windows/` | Windows Go 构建、`.syso`、图标 / Manifest / 版本资源、NSIS / MSIX 打包与签名任务 |
| `linux/Taskfile.yml`、`linux/` | Linux 原生 / Docker 构建、桌面入口 / 图标、AppImage 与 nFPM 包装及安装脚本 |
| `docker/Dockerfile.cross` | 带编译器及 GTK / WebKit 依赖的交叉构建环境；使用宿主机已构建的前端产物 |
| `appicon.png`、`appicon.icon/` | 平台图标生成输入；应用运行时托盘图标位于 `internal/desktop/icons/`，属于后端目录 |
| `darwin/`、`ios/`、`android/`、`docker/Dockerfile.server` | 保留的其他平台与 Server 模式模板，不扩大首版产品范围 |

- 根 Taskfile 通过 `GOOS` 分派平台 `build` / `package` / `run`，`PACKAGE_MANAGER` 默认 pnpm；调整本目录任务时核对调用方传入的变量、目录和依赖。
- 共用构建链安装冻结的前端依赖、生成 Binding，再运行 `build` 或 `DEV=true` 对应的 `build:dev`。保留 `go:mod:tidy`、`generate:bindings`、`build:frontend` 的 `run: once`，避免并行任务竞争修改模块文件或清理生成目录。
- `generate:bindings` 使用 `wails3 generate bindings ... -clean=true -ts -i ./...`，并传递当前 `BUILD_FLAGS` / `OBFUSCATED`。Service 位于子包，不能将扫描范围缩为应用根包；扫描入口需要的 `frontend/dist` 前置条件见根文档及现有 [Frontend CI](../.github/workflows/frontend-ci.yml)。
- 依赖、源码、配置变更需要让 Task 的 `sources` / `generates` 正确失效；不要把旧 Binding 或旧前端产物当作已验证的新构建。正式前端安装策略以其配置为准，不为模板中其他包管理器任务切换项目默认工具链。

## 2. 平台、标志与资源

- Windows 当前默认 `CGO_ENABLED=0`，使用原生 Go 交叉编译；非 Windows 主机显式启用 CGO 时走 Docker。Linux 桌面构建需要 CGO，同平台 / 同架构且有 C 编译器时走原生，其余走 Docker；SQLite 驱动纯 Go 不代表 Wails WebView 无平台依赖。
- 当前 Linux 默认栈为 GTK4 + WebKitGTK 6.0，nFPM 依赖与 Docker 编译环境应与所选栈一致；使用其他 Wails build tag 时同时核对打包依赖和目标环境，不能只改编译标志。
- 原生开发构建保留 `DEV=true` 的非 production 路径；生产构建使用 `production`、`-trimpath`、`-buildvcs=false` 与剥离符号，Windows 还使用 `-H windowsgui`。调整 flags 时同时检查原生、Docker 和 Binding 生成路径，不假定各路径传参相同；MCP 生产隔离遵守根文档。
- 图标和 `.syso` 通过 Wails 生成；Windows 构建后的根 `.syso` 会清理，检查失败构建是否遗留生成文件。`config.yml` 的 `info` / 文件关联变更会自动同步到支持平台的目标资源；`common:update:build-assets` 可按 `PLATFORM` 同步 Windows / Linux。直接运行 Wails 原生 `wails3 update build-assets` 会覆盖多个平台模板资源，先核对已有定制，再检查生成差异。
- Linux Task 生成 `linux/dan-tick.desktop`，nFPM 引用该路径与各尺寸图标；不要混淆保留的 `linux/desktop` 模板。Windows 资源路径、Manifest 与安装包元数据也应按实际任务核对，保留的占位值不能当作已确认的发行信息。
- 构建元数据由 `tools/` 在实际编译前生成，通过 `-ldflags -X` 注入 `internal/buildinfo`；Windows / Linux 原生与 Docker 路径共用版本、UTC 构建时间与 Git HEAD 定义。显示版本和 Windows 数字资源版本分别生成，不能用启动时间或文件时间冒充构建时间；`-buildvcs=false` 不提供自动 VCS 元数据。具体 CI / Tag 规则见 [构建与发布](../docs/构建与发布.md)。

## 3. 构建与打包验证

以下命令在仓库根目录执行，按本次变更的平台和任务选择：

```sh
wails3 build                               # 当前平台生产构建
wails3 build DEV=true                      # 原生开发构建路径
wails3 build GOOS=windows                   # Windows 构建
wails3 build GOOS=linux                     # Linux 构建
wails3 task package GOOS=windows            # 默认 NSIS；MSIX 按 Taskfile 参数选择
wails3 task package GOOS=linux              # Linux 打包任务
```

- Taskfile、编译标志或资源路径变更验证受影响构建；触及打包配置时再验证对应安装包产物、资源和依赖。不为纯指令文档修改执行完整打包。
- 跨平台构建成功不代表目标平台运行通过；WebView、托盘、窗口、安装 / 卸载与 Keyring 的验收仍需对应平台人工验证。环境或工具缺失时如实记录未完成的检查。
- 构建后按根文档检查工作树，特别核对 `go.mod` / `go.sum`、生成 Binding / 前端产物、图标、`.desktop`、`.syso` 和安装器辅助文件；只纳入本次明确需要的源码 / 配置 / 资源变更。
