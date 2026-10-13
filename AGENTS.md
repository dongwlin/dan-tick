# dan-tick 开发指南（AI / Coding Agent）

本文件适用于整个仓库；子目录指令只补充其目录范围内的约定。

## 1. Scoped Instructions（目录作用域）

修改子目录中的文件前，先读取修改路径涉及的子目录 `AGENTS.md`，并同时遵守本文件。仅加载本次任务涉及的目录指令，不要求每次读取所有子目录文档；跨目录修改时分别读取对应指令。

| 指令文件 | 适用范围 |
| --- | --- |
| [`internal/AGENTS.md`](internal/AGENTS.md) | `internal/` 及其子目录：Go 后端的包职责、数据与 Binding 边界、生命周期和测试 |
| [`build/AGENTS.md`](build/AGENTS.md) | `build/` 及其子目录：Wails / Task 构建链、平台资源与打包验证 |
| [`frontend/AGENTS.md`](frontend/AGENTS.md) | `frontend/` 及其子目录：正式 Vue 前端的架构、UI、工具链与验证 |
| [`prototype/AGENTS.md`](prototype/AGENTS.md) | `prototype/` 及其子目录：独立 Vanilla TypeScript 演示原型 |

各子目录指令只作用于表中范围，互不自动适用。未设局部指令的目录遵守根文档及任务相关规范；不要把原型的实现方式或工作流程套用到正式前端。根 `Taskfile.yml`、`go.mod` 和 `.github/workflows/` 不在 `build/` 的作用域内；修改构建链时按需参考构建指令及相关脚本，跨目录修改仍分别加载对应指令。

## 2. 项目与文档入口

- dan-tick 是面向 **Windows、Linux** 的 B 站直播弹幕定时发送桌面应用，本地管理直播间、任务、调度、执行记录与账号凭证，不依赖浏览器发送弹幕。
- **需求与验收标准**以 [`docs/PRD.md`](docs/PRD.md) 为准；**技术决策及通用规范的项目例外**以 [`docs/技术栈选型.md`](docs/技术栈选型.md) 为准。后续待办见 [`docs/待办.md`](docs/待办.md)，版本方向见 [`docs/ROADMAP.md`](docs/ROADMAP.md)。
- 按任务阅读 [`docs/specs/`](docs/specs/) 中相关 Pack 及关联规范；后端与正式前端的规范入口见各自子目录指令。修改根入口时使用 [Wails 桌面后端 Pack](docs/specs/packs/Wails%20桌面后端%20Pack.md)。`docs/specs/` 是外部 specs 仓库复制件，来源见 [manifest](docs/specs/spec-pack-manifest.json)；项目差异写入技术选型文档，不直接编辑复制的规范。
- 根 [README](README.md) 是项目入口，子目录 README 提供对应运行说明。实现状态以实际代码及技术选型核对，区分已实现行为与计划行为。

## 3. 项目结构与全局架构边界

| 路径 | 职责 |
| --- | --- |
| `main.go` | 嵌入 `frontend/dist`，将资源交给 `internal/app.Run`，并使用返回的退出码结束进程 |
| `internal/` | Go 应用装配、桌面 Service 与业务实现；包职责和后端约定见局部指令 |
| `frontend/` | 正式 Vue 界面；具体结构与实现约定见局部指令 |
| `prototype/` | 独立演示原型，使用会话演示数据 |
| `Taskfile.yml`、`build/` | Wails / Task 的开发、构建和各平台打包配置；`build/config.yml` 为开发配置 |
| `docs/` | PRD、技术决策、待办、数据模型与引入的工程规范 |

- 正式应用中，**Go 是权威业务状态来源**，负责调度、数据库、HTTP 发送、频率限制、凭证和进程生命周期；前端承担配置、展示与交互，不另行维护任务、调度等权威业务状态。调用使用 **Wails Service Binding**，后端主动通知使用 **Wails Events**，不要另建桌面业务 HTTP API。
- Go 模块为 `github.com/dongwlin/dan-tick`，Go 与 Wails 版本要求以 `go.mod` 为准；前后端 Runtime 与构建 CLI 的版本须与项目依赖匹配。
- Go 应用源码根目录仅保留入口 `main.go`，包括 `frontend/dist` 的 `go:embed` 声明；嵌入路径相对 Go 包目录，资源由根入口通过 `fs.FS` 传入应用组合根。其余后端代码按职责放在 `internal/`，`build/` 的平台模板 Go 文件不受入口限制。
- 调度器只触发计划；执行防重、持久化、漏发判定与恢复由业务逻辑和 SQLite 事务承担。任务使用明确时区（PRD 默认 `Asia/Shanghai`）；错过计划默认不补发，手动执行不改变定时计划。同一账号的多个任务共享发送频率限制。
- B 站接口成功不能只看 HTTP 200，需校验业务响应；发送结果不确定时默认不自动重试。HTTP 请求必须有超时与取消能力，避免停任务或退出后继续发送。
- 凭证只进入系统安全存储；不可用时明确失败，不回退明文配置。不要记录 Cookie，或将其放入普通前端 Store / Query 缓存、持久化 Web 存储及路由；B 站鉴权与请求统一由 Go 完成。旧双字段 Keyring 数据保留并要求重扫，未完成轮换的会话拒绝发送。
- `frontend/` 与 `prototype/` 分别管理依赖与锁文件，不合并 workspace。不要手动编辑或提交 `frontend/bindings/`、`frontend/dist/`；它们由 Wails / Vite 生成，后端 Service 或构建流程变更也须遵守此约束。
- `build/` 中的 macOS、iOS、Android 模板不代表本项目支持这些平台；目标平台以 PRD 的 Windows、Linux 为准。

## 4. 构建与真实桌面调试

运行桌面应用需 Go、Node / pnpm、Wails 3 CLI 及目标平台构建依赖；正式前端安装与工具链见 [前端指令](frontend/AGENTS.md)。

在仓库根目录执行：

```sh
wails3 dev                 # 桌面开发，前端热更新
wails3 build               # 构建当前平台桌面应用
WAILS_MCP=1 wails3 dev     # 开发版启用 Wails 应用 MCP service
```

- Wails 构建通过 `build/Taskfile.yml` 安装前端依赖、生成 Binding 并构建前端；共享构建链必须继续扫描 `./...`，不能因 Service 位于 `internal/desktop/` 而仅扫描根包。流程可能运行 `go mod tidy`，构建后检查 `go.mod` / `go.sum` 有无意外变化；平台任务、标志与资源约定见 [构建指令](build/AGENTS.md)。
- **运行全仓 Go 检查或扫描应用入口前须有 `frontend/dist/`**，否则 `main.go` 的 `go:embed` 会报错；可先通过 Wails 构建准备前端产物。干净检出的 Binding 生成前置处理见前端指令及现有 CI。

### 调试真实桌面窗口

涉及 Binding、Events、原生窗口行为或真实 WebView 交互时，使用运行中开发版的 [Wails 应用 MCP service](https://v3.wails.io/zh-cn/guides/mcp-service/)。普通 Vite 浏览器页面没有 Wails bridge，不能替代桌面验证；`wails3 mcp` 管理项目生命周期，不检查运行中的 WebView。

- `WAILS_MCP=1 wails3 dev` 通过 `EXTRA_TAGS` 加入 `mcp` build tag。Windows PowerShell 先设置 `$env:WAILS_MCP = "1"`（`WAILS_MCP_PORT` 同理）。
- 启动日志打印 Streamable HTTP MCP endpoint，通常为 `http://127.0.0.1:9099/mcp`，以实际日志为准；端口被占用时使用 `WAILS_MCP=1 WAILS_MCP_PORT=0 wails3 dev` 选择空闲端口。MCP 端口与 `wails3 dev -port <vite-port>` 的 Vite 端口独立。
- 先用 `app_info` / `windows_list` 确认窗口，再用 `dom_query`、`dom_html`、`screenshot_dom` 检查结构；用 `mouse_click`、`keyboard_type`、`keyboard_press` 复现交互，通过 `selector: "#id"` 或视口 `x` / `y` 定位。鼠标工具显示动画光标，可用 `WAILS_MCP_HIDE_CURSOR=1` 关闭。
- Binding / Events 可用 `call_bound_method`、`emit_event`、`wait_for_event` 验证；`js_eval` 会执行任意代码并可能改变状态，仅在必要时使用。`screenshot_dom` 是含视口、滚动位置与元素轮廓的结构化快照；视觉外观仍需真实窗口观察或系统截图。
- MCP service 是实验性本地开发工具，可读取 DOM、执行 JavaScript、点击与调用 Go 方法。保持默认 loopback 绑定；未设置 `WAILS_MCP_TOKEN` 时本机进程与环回浏览器来源会被信任，需要更强隔离时设置高熵令牌，非 loopback 绑定必须设置令牌。不要把 `mcp` tag 加入生产构建；默认 `wails3 build` 不含服务器代码，显式设置 `WAILS_MCP=1` 才会加入。
- 调试结束后停止开发版。开发版与打包版共用单实例 ID `io.github.dongwlin.dan-tick` 和用户数据目录；遗留实例会使后续启动只唤醒旧实例并退出。

## 5. 通用工作流与验证

1. 读取对应 PRD 条目、技术选型及任务相关 Pack，确认修改路径的局部指令、Go / Binding / 界面边界和平台影响。
2. 用最小可维护实现满足场景，按功能渐进引入；不为未来需求提前拆层、创建空架构或添加无关防御分支。命名与组织遵循相关规范及相邻模块，不从模板偶然风格推导硬性规则，不借功能变更全面重排无关代码。
3. 为新增或修改的**关键行为**补测试，不机械追求覆盖率。自动测试不得访问真实 B 站生产接口或使用实际账号凭证。
4. 影响托盘、窗口隐藏、单实例、自启动、Keyring、睡眠恢复或后台运行时，记录对应平台的人工验证；Go 单元测试及普通浏览器测试不能替代桌面验收。

按改动类型验证：

- **Go 代码**：修改文件执行 `gofmt`，先跑受影响测试；准备好前端产物后运行 `go test ./...`、`go vet ./...`，并发敏感代码在环境支持时运行 `go test -race ./...`。后端测试边界和重点场景见 `internal/` 局部指令；根入口或平台模板的变更也执行适用的 Go 检查。
- **正式前端代码**：按 [`frontend/AGENTS.md`](frontend/AGENTS.md) 执行前端检查；影响 Wails 集成或桌面行为时，再执行 `wails3 build` 和相关平台手动检查。纯 Go 或其他目录任务不必加载前端实现细节，但仍需满足上述跨目录构建前置条件。
- **纯文档**：核对 Markdown 链接、路径与命令真实性；不为纯文档或简单模板调整搭建完整测试框架。
- **所有改动**：完成前运行 `git diff --check`、`git status --short`，排查空白错误、生成文件、凭证与无关改动。无法运行的检查如实说明，不得报告为通过；保留用户未提交改动，未经要求不自行提交、重置或清理。

## 6. Git 分支与提交

- **Commit message 使用英文并符合 Conventional Commits**：`<type>[optional scope]: <description>`，如 `docs: refine agent development guide`。每个 Commit 只覆盖一个明确目的，不混入无关重构。
- **Branch 名称符合 `<type>/<kebab-case-description>`**，type 与主要修改类型一致，如 `test/agent-server-callback-test`；禁止使用 Agent、模型或工具名称作为 prefix。
- 优先采用项目 Development Guide 定义的 change type vocabulary。目前没有该指南或专属类型词表，在补充前，分支 type 与对应的 Conventional Commits 类型保持一致；若后续新增指南，以其词表为准。
- 不从少量提交推导固定 scope、合并策略或分支流程；分支操作与提交遵守用户当前任务要求。
