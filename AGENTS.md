# dan-tick 开发指南（AI / Coding Agent）

## 1. 先了解项目和当前阶段

- dan-tick 是面向 Windows、Linux 的 B 站直播弹幕定时发送桌面应用：本地管理直播间、任务、调度、执行记录与账号凭证，不依赖浏览器发送弹幕。
- **需求与验收标准**以 [`docs/PRD.md`](docs/PRD.md) 为准；**技术决策及对通用规范的例外**以 [`docs/技术栈选型.md`](docs/技术栈选型.md) 为准。实现前按任务阅读 [`docs/specs/`](docs/specs/) 中的 [Vue SPA Pack](docs/specs/packs/Vue%20SPA%20Pack.md)、[亚克力设计 Pack](docs/specs/packs/亚克力设计%20Pack.md) 或 [Wails 桌面后端 Pack](docs/specs/packs/Wails%20桌面后端%20Pack.md) 及其关联规范。
- 开发顺序和最小可提交单元见 [`docs/开发计划.md`](docs/开发计划.md)，它用于安排实施步骤，不覆盖 PRD 与技术选型的权威性；UI 原型可以独立继续迭代。
- **正式应用当前仍是 Wails 3 模板，并未实现 PRD 的业务功能。** `prototype/` 已有独立的 Vanilla TypeScript UI 演示，全部使用模拟状态，与 `frontend/` 正式 Vue 前端分开。技术栈选型中的 Bun、SQLite、Cron、Pinia、Tailwind、路由、表单和测试框架等是后续落地方案，不代表正式应用已安装这些依赖或已有对应目录。按功能渐进引入，不为未来需求创建空架构。
- `docs/specs/` 是外部 specs 仓库的复制件，来源信息见 `docs/specs/spec-pack-manifest.json`。项目差异写入技术选型文档；不要直接编辑复制的规范。根目录 README 是项目入口，`frontend/README.md` 仍是 Vue 模板说明，应以代码和工程配置为准。

## 2. 现有结构与调用边界

| 路径 | 目前职责 |
| --- | --- |
| `main.go` | Wails 应用初始化、窗口、Service 注册、事件发送；通过 `//go:embed all:frontend/dist` 打包前端 |
| `greetservice.go` | 示例 `GreetService`，不是弹幕业务服务 |
| `frontend/src/main.ts`、`App.vue`、`components/HelloWorld.vue` | Vue 入口及模板页面；使用 `<script setup lang="ts">` 和 Composition API |
| `frontend/public/`、`frontend/index.html` | 当前模板的全局样式、图片、字体及 HTML 入口 |
| `frontend/package.json`、`vite.config.ts`、`tsconfig.json` | 前端脚本、Vite + Wails Binding 插件、TypeScript 配置 |
| `prototype/` | 独立 UI 原型；详细职责和运行命令见 `prototype/AGENTS.md`、`prototype/README.md` |
| `Taskfile.yml`、`build/` | Wails 3 / Task 的开发、构建和各平台打包配置；`build/config.yml` 为 Wails 开发配置 |
| `docs/PRD.md`、`docs/技术栈选型.md`、`docs/specs/` | 产品规则、项目决策与引入的工程规范 |

- 当前 Go 模块名还是 `changeme`（见 `go.mod`），模板前端通过生成的 `frontend/bindings/changeme` 调用 Go Service，并通过 Wails Events 接收时间事件。不要手动编辑 `frontend/bindings/` 或 `frontend/dist/`；它们由 Wails / Vite 生成且当前未提交。
- 业务落地后，**Go 是权威业务状态来源**：处理调度、数据库、HTTP 发送、频率限制、凭证、进程生命周期；前端只承担配置、展示与交互。前端调用使用 **Wails Service Binding**，Go 主动通知使用 **Wails Events**，不要为桌面业务另建 HTTP API。
- 未来前端按 Feature 聚合业务，Page 负责组合及解析路由参数，共享组件不能反向依赖 Feature；保持小模块扁平，有实际复用需求再提取。当前还没有这些业务目录，创建时依照 `docs/specs/` 的架构规范。
- `build/` 中也保留了 macOS、iOS 和 Android 的模板打包文件，但 PRD 首版目标平台仅为 Windows、Linux；不要把模板存在等同于要求支持这些平台。

## 3. 开发环境与可执行命令

- `go.mod` 要求 **Go 1.25.0**；当前框架依赖是 `github.com/wailsapp/wails/v3 v3.0.0-beta.25`。前端由 **Vue 3 + TypeScript + Vite 8** 构成，仓库提供 Wails 3 CLI / Taskfile 流程。
- 当前 `Taskfile.yml` 的 `PACKAGE_MANAGER` 默认是 `npm`；`frontend/package.json` 只声明了 npm scripts，**仓库尚无前端锁文件**。不要因为通用 specs 提到其他包管理器，就假设已完成切换。
- `frontend/.npmrc` 声明了 `minimum-release-age=10080`，但其中明确注明 npm 尚不支持这项限制；不要将它误认为当前 npm 安装的安全保证。Vite 开发服务器默认绑定 `127.0.0.1:9245`（可通过 `WAILS_VITE_PORT` 调整）。

```sh
# 仓库根目录；需先安装 Go、Node/npm、Wails 3 CLI 及目标平台所需的 Wails 构建依赖
cd frontend && npm install
cd ..
wails3 dev                 # 桌面开发，前端热更新
wails3 build               # 构建当前平台桌面应用

# 仅前端检查 / 构建（需要已生成 Wails bindings）
cd frontend
npm run build              # vue-tsc + Vite production build
npm run build:dev          # vue-tsc + 非压缩的 development build
```

- Wails 构建任务会通过 `build/Taskfile.yml` 安装前端依赖、生成 Binding、构建前端，并可能运行 `go mod tidy`；构建后检查有无意外的 `go.mod` / `go.sum` 变化。
- Go 变更：对修改的 Go 文件执行 `gofmt`，并按需运行 `go test ./...`、`go vet ./...`；并发敏感代码还需运行 `go test -race ./...`（环境支持时）。**独立运行 Go 命令前须有 `frontend/dist/`**，否则 `main.go` 的 `go:embed` 会报错。可以先通过 Wails 构建生成前端产物。
- **目前没有测试文件、CI 工作流、前端 `test` / `lint` / 独立 `typecheck` 脚本，也没有已接入的 ESLint / Prettier 配置。** 类型检查包含在 `npm run build` 中；不要编造 `npm run test`、`npm run lint` 或把 specs 中规划的检查当作当前可运行命令。工具落地时再补相应脚本与 CI。

## 4. 新代码的架构与风格约束

- Go 后端按需采用 **Bun + `modernc.org/sqlite` + Bun Migrations**、zerolog、`robfig/cron/v3`、`go-keyring`、`net/http` 和 testify。优先具体类型与清晰的 Service / 业务边界，不引入 Gin、本地 REST Server、分布式调度或没有实际需求的 Repository 接口。
- Go 应用源码以根目录仅保留 `main.go` 为目标；现有 `greetservice.go` 是待移除的模板文件。新后端业务代码按职责放在 `internal/`，不预建空包。模型复用、Binding Input、事务和依赖方向以 [Go 桌面应用架构约定](docs/specs/profiles/backend/wails-desktop/Go%20桌面应用架构约定.md) 为准；数据库初稿见 [后端数据模型设计](docs/后端数据模型设计.md)。`build/` 的平台模板 Go 文件不属于根目录限制。
- 调度器只负责触发计划；执行防重、持久化、漏发判定和恢复由业务逻辑与 SQLite 事务承担。任务使用明确时区（PRD 默认 `Asia/Shanghai`）；错过计划默认不补发，手动执行不改变定时计划。同一账号的多个任务共享发送频率限制。
- B 站接口成功不能只看 HTTP 200，需校验业务响应；发送结果不确定时默认不自动重试。HTTP 请求要有超时、取消能力，避免在停任务或退出后继续发送。
- 凭证只进入系统安全存储；不可用时明确失败，不回退到明文配置。不要记录 Cookie、将其放入普通前端 Store / Query 缓存、持久化 Web 存储或路由；B 站鉴权与请求统一在 Go 后端完成。
- 前端目标选型：**Vue Router 4 Hash History**、Tailwind CSS v4、Reka UI、`lucide-vue-next` 与亚克力设计系统；Pinia 只管理必要的客户端共享状态，TanStack Query 缓存从 Binding 获取的业务数据，Events 触发相关查询失效，不复制 Go 业务状态。简单表单用本地 Vue 状态，任务编辑使用 vee-validate + Zod；后端仍负责最终校验。
- 样式兼顾 Windows / Linux WebView 可用性；原生毛玻璃不可用时提供可读的实色回退。交互优先复用 Reka UI 并遵循 `docs/specs/` 的可访问性规范。
- **现有命名证据有限**：Vue 组件文件是 `App.vue`、`HelloWorld.vue` 这样的 PascalCase，Go 文件遵循 Go 命名与 `gofmt`；模板 TS 代码的引号、空格尚不统一。新代码以引入的规范及其相邻模块为准，不要把模板偶然风格提升为项目硬性规则，也不要借功能变更全面重排现有代码。

## 5. 变更工作流与测试策略

1. 读对应 PRD 条目、技术选型和相关 Pack，区分已有行为与计划行为；确认需要修改的 Go、Binding、前端边界及平台影响。
2. 先用最小可维护实现满足场景；不要为了“未来扩展”提前拆层、重复维护业务状态或加与任务无关的防御分支。
3. 为新增或修改的**关键行为**补测试，而不是机械追求覆盖率：Go 侧用 `testing` / testify、可控时钟、真实临时 SQLite（包括重新打开后的恢复）及 `httptest` 验证调度防重、时区、状态变更、停止条件、限流和异常响应。测试不得访问真实 B 站生产接口或使用实际账号凭证。
4. 前端测试基础设施**引入之后**，默认用 Vitest + Vue Testing Library 验证用户可见行为；对 Binding / Events 外部边界做替换，不断言内部实现。真实焦点、键盘或 WebView 差异按需做浏览器和 Windows / Linux 实机验证；不要为纯文档或简单模板调整搭建完整测试框架。
5. 影响托盘、窗口隐藏、单实例、自启动、Keyring、睡眠恢复或后台运行的变更，记录对应平台的人工验证；Go 单元测试和普通浏览器测试不能替代桌面验收。

提交改动前至少检查：

- `git diff --check`、`git status --short`：确认无空白错误、生成文件、凭证或无关改动。
- Go 代码改动：`gofmt` 后执行受影响测试；准备好前端构建产物后运行 `go test ./...`、`go vet ./...`，并发敏感场景酌情加 `-race`。
- 前端代码改动：生成 / 更新 Binding 后在 `frontend/` 运行 `npm run build`；影响 Wails 集成或桌面行为时再执行 `wails3 build` 和相关平台手动检查。
- 纯文档改动：核对 Markdown 链接、路径与命令的真实性即可。无法运行的检查需说明原因，不得报告为通过。保留用户未提交改动；未经要求不自行提交、重置或清理。

## 6. Git 分支与提交

- **Commit message 必须使用英文，且符合 Conventional Commits**：`<type>[optional scope]: <description>`，例如 `docs: refine agent development guide`。每个 Commit 只覆盖一个明确的变更目的，不混入无关重构。
- **Branch 名称必须符合 `<type>/<kebab-case-description>`**，`type` 与主要修改类型一致；例如 `test/agent-server-callback-test`。禁止使用 Agent、模型或工具名称作为 branch prefix。
- **优先采用项目 Development Guide 定义的 change type vocabulary**。当前仓库没有 Development Guide 或已定义的类型词表，因此不要声称已有专属类型清单；在补充该指南前，分支 `type` 与对应的 Conventional Commits 类型保持一致。若后续新增 Development Guide，以其中的词表为准。
- 不要从少量现有提交推导固定的 Commit scope、合并策略或分支流程；分支操作与提交应遵守用户当前任务要求。
