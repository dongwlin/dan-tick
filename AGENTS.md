# dan-tick 开发指南（AI / Coding Agent）

## 1. 先了解项目和当前阶段

- dan-tick 是面向 Windows、Linux 的 B 站直播弹幕定时发送桌面应用：本地管理直播间、任务、调度、执行记录与账号凭证，不依赖浏览器发送弹幕。
- **需求与验收标准**以 [`docs/PRD.md`](docs/PRD.md) 为准；**技术决策及对通用规范的例外**以 [`docs/技术栈选型.md`](docs/技术栈选型.md) 为准。实现前按任务阅读 [`docs/specs/`](docs/specs/) 中的 [Vue SPA Pack](docs/specs/packs/Vue%20SPA%20Pack.md)、[亚克力设计 Pack](docs/specs/packs/亚克力设计%20Pack.md) 或 [Wails 桌面后端 Pack](docs/specs/packs/Wails%20桌面后端%20Pack.md) 及其关联规范。
- 开发顺序和最小可提交单元见 [`docs/开发计划.md`](docs/开发计划.md)，它用于安排实施步骤，不覆盖 PRD 与技术选型的权威性；UI 原型可以独立继续迭代。
- **正式外壳已按 F3 迁移，业务页面尚未迁移，尚不能通过界面发送弹幕。** 已接入 Bun、SQLite、首次 Migration 与基础存储测试；`prototype/` 仍是独立的 Vanilla TypeScript 模拟 UI，与正式 Vue 前端分开。F1-a 已将 Cron 调度、启动恢复与共享 Sender / Executor 装配进 Wails 生命周期，直播间 / 任务 Binding 留待 F1-b；E4 已提供 SQLite 执行历史查询。前端已建立 Vue Router 4（Hash History）、正式图标侧栏和自绘标题栏、Tailwind v4 主题视觉基础、Reka UI / Lucide 交互原语以及 Vitest 行为测试；Pinia、业务表单等尚未接入。按功能渐进引入，不为未来需求创建空架构。
- `docs/specs/` 是外部 specs 仓库的复制件，来源信息见 `docs/specs/spec-pack-manifest.json`。项目差异写入技术选型文档；不要直接编辑复制的规范。根目录 README 是项目入口，`frontend/README.md` 说明正式 Vue 前端的工具链与构建流程。

## 2. 现有结构与调用边界

| 路径 | 目前职责 |
| --- | --- |
| `main.go` | 启动时打开并迁移数据库、退出时关闭连接；Wails 应用、窗口、生命周期钩子及前端嵌入 |
| `internal/app/` | Keyring、共享 Sender / Executor / Scheduler 的业务组合根及启动 / 退出收尾 |
| `internal/storage/` | 用户数据目录中的 SQLite 连接、Bun Migration、首版六张表及真实数据库测试 |
| `internal/history/` | E4 执行历史的分页列表、详情及发送尝试查询；不读取当前任务定义重建历史 |
| `frontend/src/main.ts`、`src/app/`、`src/pages/` | Vue 入口、应用装配（根组件与集中路由表，Vue Router 4 Hash History）及占位页面；使用 `<script setup lang="ts">` 和 Composition API |
| `frontend/src/style.css`、`src/app/theme.ts` | Tailwind v4 CSS-first 语义令牌、亚克力材质及深浅 / 跟随系统 UI 主题；仅保存 `dan-tick-theme` 偏好，存储故障提供可见反馈 |
| `frontend/src/components/UiButton.vue` | 当前跨页面复用的按钮 / RouterLink 样式，保留原生动作与导航语义；Reka RadioGroup 当前直接用于设置页 |
| `frontend/public/`、`frontend/index.html` | 静态资源及 HTML 入口 |
| `frontend/package.json`、`pnpm-lock.yaml`、`pnpm-workspace.yaml`、`.node-version` | 前端脚本、固定工具链、依赖锁文件及 pnpm 安装策略 |
| `frontend/vite.config.ts`、`tsconfig.json`、`tsconfig.node.json` | Vite + Wails Binding 插件、源码与构建配置的 TypeScript 检查 |
| `frontend/vitest.config.ts`、`src/test/setup.ts`、`*.test.ts` | 独立于 Wails 的 Vitest + happy-dom 测试配置、测试环境清理与用户可观察行为测试 |
| `frontend/eslint.config.mjs`、`.prettierrc.json`、`.prettierignore` | 正式前端 ESLint Flat Config、格式约定与依赖 / 生成物忽略规则 |
| `prototype/` | 独立 UI 原型；详细职责和运行命令见 `prototype/AGENTS.md`、`prototype/README.md` |
| `Taskfile.yml`、`build/` | Wails 3 / Task 的开发、构建和各平台打包配置；`build/config.yml` 为 Wails 开发配置 |
| `docs/PRD.md`、`docs/技术栈选型.md`、`docs/specs/` | 产品规则、项目决策与引入的工程规范 |

- Go 模块名为 `github.com/dongwlin/dan-tick`（见 `go.mod`）；正式 Wails 模板演示 Service / 时间事件已移除，业务 Binding 将在 F1-b 接入。不要手动编辑 `frontend/bindings/` 或 `frontend/dist/`；它们由 Wails / Vite 生成且当前未提交。
- 业务落地后，**Go 是权威业务状态来源**：处理调度、数据库、HTTP 发送、频率限制、凭证、进程生命周期；前端只承担配置、展示与交互。前端调用使用 **Wails Service Binding**，Go 主动通知使用 **Wails Events**，不要为桌面业务另建 HTTP API。
- 未来前端按 Feature 聚合业务，Page 负责组合及解析路由参数，共享组件不能反向依赖 Feature；保持小模块扁平，有实际复用需求再提取。当前还没有这些业务目录，创建时依照 `docs/specs/` 的架构规范。
- `build/` 中也保留了 macOS、iOS 和 Android 的模板打包文件，但 PRD 首版目标平台仅为 Windows、Linux；不要把模板存在等同于要求支持这些平台。

## 3. 开发环境与可执行命令

- `go.mod` 要求 **Go 1.25.0**；当前框架依赖是 `github.com/wailsapp/wails/v3 v3.0.0-beta.25`。前端由 **Vue 3 + TypeScript + Vite 8** 构成，仓库提供 Wails 3 CLI / Taskfile 流程。
- 正式前端使用 **pnpm 11.28.4**，由 `frontend/package.json` 的 `packageManager` 固定；`frontend/.node-version` 指定 **Node.js 24.18.0**，`engines` 要求 `>=24.18.0 <25`。`Taskfile.yml` 的 `PACKAGE_MANAGER` 默认是 `pnpm`，安装使用已提交的 `frontend/pnpm-lock.yaml` 与 `pnpm install --frozen-lockfile`。`prototype/` 独立管理依赖，不合并 workspace。
- `frontend/pnpm-workspace.yaml` 保存单包安装配置，启用 `minimumReleaseAge: 10080`（7 天）及 `engineStrict: true`；不再用旧 `.npmrc` 表达这些策略。Vite 开发服务器默认绑定 `127.0.0.1:9245`（可通过 `WAILS_VITE_PORT` 调整）。源码别名为 `@/*` → `src/*`、`@bindings/*` → `bindings/*`，TypeScript 与 Vite 保持一致。

```sh
# 仓库根目录；需先安装 Go、Node/pnpm、Wails 3 CLI 及目标平台所需的 Wails 构建依赖
cd frontend && pnpm install --frozen-lockfile
cd ..
wails3 dev                 # 桌面开发，前端热更新
wails3 build               # 构建当前平台桌面应用

# 前端检查 / 构建（类型检查和构建需要已生成 Wails bindings）
cd frontend
pnpm run test              # Vitest + Testing Library 行为测试，不需要运行 Wails
pnpm run typecheck         # 独立检查 Vue 源码和 Vite 配置的类型
pnpm run lint              # ESLint 检查；不允许 warning
pnpm run format:check      # Prettier 格式检查
pnpm run lint:fix          # 修复 ESLint 可自动修复的问题
pnpm run format            # 按统一约定格式化正式前端
pnpm run build             # Vue 源码和 Vite 配置类型检查 + production build
pnpm run build:dev         # 同上类型检查 + 非压缩的 development build
```

- Wails 构建任务会通过 `build/Taskfile.yml` 安装前端依赖、生成 Binding、构建前端，并可能运行 `go mod tidy`；构建后检查有无意外的 `go.mod` / `go.sum` 变化。
- Go 变更：对修改的 Go 文件执行 `gofmt`，并按需运行 `go test ./...`、`go vet ./...`；并发敏感代码还需运行 `go test -race ./...`（环境支持时）。**独立运行 Go 命令前须有 `frontend/dist/`**，否则 `main.go` 的 `go:embed` 会报错。可以先通过 Wails 构建生成前端产物。
- **已有 Go 集成测试、前端 `pnpm run test`、ESLint / Prettier 和独立 `typecheck`，尚无 CI 工作流。** Vitest 通过 `frontend/vitest.config.ts` 使用 Vue 插件与 happy-dom，实际测试位于 `src/app/App.test.ts`、`src/pages/SettingsPage.test.ts`；不启动 Wails 桌面插件。`typecheck` 通过 `vue-tsc` 检查 Vue 源码与生成 Binding、通过 `tsc` 检查 `vite.config.ts` 和 `vitest.config.ts`；`build` 和 `build:dev` 均先执行 `typecheck`。测试覆盖随真实业务功能逐步增长，CI 留待 FE9。
- ESLint 采用 Flat Config，启用 JavaScript、TypeScript 类型感知、Vue 与 Vue 可访问性推荐规则，以及重复导入、类型导入和非空断言检查；`lint` / `lint:fix` 使用 `--max-warnings 0`。Prettier 独立执行，`eslint-config-prettier` 最后关闭冲突规则，不通过 ESLint 执行格式化。检查限定 `frontend/`，忽略依赖、生成 Binding、构建与覆盖率产物；Prettier 另外忽略锁文件、第三方 SVG 和许可证。

## 4. 新代码的架构与风格约束

- Go 后端已采用 **Bun + `modernc.org/sqlite` + Bun Migrations** 与 testify，已按业务需求接入 zerolog、`robfig/cron/v3`、`go-keyring` 和 `net/http`；F1-a 已完成正式桌面应用的调度与执行装配，业务 Binding 尚待 F1-b。优先具体类型与清晰的 Service / 业务边界，不引入 Gin、本地 REST Server、分布式调度或没有实际需求的 Repository 接口。
- Go 应用源码根目录仅保留 `main.go`，模板 `greetservice.go` 已在 F1-a 移除。新后端业务代码按职责放在 `internal/`，不预建空包。模型复用、Binding Input、事务和依赖方向以 [Go 桌面应用架构约定](docs/specs/profiles/backend/wails-desktop/Go%20桌面应用架构约定.md) 为准；数据库初稿见 [后端数据模型设计](docs/后端数据模型设计.md)。`build/` 的平台模板 Go 文件不属于根目录限制。
- 调度器只负责触发计划；执行防重、持久化、漏发判定和恢复由业务逻辑与 SQLite 事务承担。任务使用明确时区（PRD 默认 `Asia/Shanghai`）；错过计划默认不补发，手动执行不改变定时计划。同一账号的多个任务共享发送频率限制。
- B 站接口成功不能只看 HTTP 200，需校验业务响应；发送结果不确定时默认不自动重试。HTTP 请求要有超时、取消能力，避免在停任务或退出后继续发送。
- 凭证只进入系统安全存储；不可用时明确失败，不回退到明文配置。不要记录 Cookie、将其放入普通前端 Store / Query 缓存、持久化 Web 存储或路由；B 站鉴权与请求统一在 Go 后端完成。
- 前端目标选型：**Vue Router 4 Hash History**、Tailwind CSS v4、Reka UI、`lucide-vue-next` 与亚克力设计系统；Pinia 只管理必要的客户端共享状态，TanStack Query 缓存从 Binding 获取的业务数据，Events 触发相关查询失效，不复制 Go 业务状态。简单表单用本地 Vue 状态，任务编辑使用 vee-validate + Zod；后端仍负责最终校验。
- 样式兼顾 Windows / Linux WebView 可用性；原生毛玻璃不可用时提供可读的实色回退。交互优先复用 Reka UI 并遵循 `docs/specs/` 的可访问性规范。
- **现有命名证据有限**：Vue 组件文件是 `App.vue`、`HelloWorld.vue` 这样的 PascalCase，Go 文件遵循 Go 命名与 `gofmt`。正式前端的 Prettier 约定为单引号、无分号、2 空格缩进、100 字符行宽、LF 和 `trailingComma: all`；其他命名与组织按引入的规范及相邻模块，不要把模板偶然风格提升为项目硬性规则，也不要借功能变更全面重排无关代码。

## 5. 变更工作流与测试策略

1. 读对应 PRD 条目、技术选型和相关 Pack，区分已有行为与计划行为；确认需要修改的 Go、Binding、前端边界及平台影响。
2. 先用最小可维护实现满足场景；不要为了“未来扩展”提前拆层、重复维护业务状态或加与任务无关的防御分支。
3. 为新增或修改的**关键行为**补测试，而不是机械追求覆盖率：Go 侧用 `testing` / testify、可控时钟、真实临时 SQLite（包括重新打开后的恢复）及 `httptest` 验证调度防重、时区、状态变更、停止条件、限流和异常响应。测试不得访问真实 B 站生产接口或使用实际账号凭证。
4. 前端默认用 Vitest + Vue Testing Library + user-event 验证用户可见行为；对 Binding / Events 外部边界做替换，不断言内部实现。真实焦点、键盘或 WebView 差异按需做浏览器和 Windows / Linux 实机验证；不要为纯文档或简单模板调整搭建完整测试框架。
5. 影响托盘、窗口隐藏、单实例、自启动、Keyring、睡眠恢复或后台运行的变更，记录对应平台的人工验证；Go 单元测试和普通浏览器测试不能替代桌面验收。

提交改动前至少检查：

- `git diff --check`、`git status --short`：确认无空白错误、生成文件、凭证或无关改动。
- Go 代码改动：`gofmt` 后执行受影响测试；准备好前端构建产物后运行 `go test ./...`、`go vet ./...`，并发敏感场景酌情加 `-race`。
- 前端代码改动：在 `frontend/` 运行 `pnpm run test`、`pnpm run lint`、`pnpm run format:check`；生成 / 更新 Binding 后运行 `pnpm run build`，可用 `pnpm run typecheck` 独立检查类型，构建仍包含该检查。影响 Wails 集成或桌面行为时再执行 `wails3 build` 和相关平台手动检查。
- 纯文档改动：核对 Markdown 链接、路径与命令的真实性即可。无法运行的检查需说明原因，不得报告为通过。保留用户未提交改动；未经要求不自行提交、重置或清理。

## 6. Git 分支与提交

- **Commit message 必须使用英文，且符合 Conventional Commits**：`<type>[optional scope]: <description>`，例如 `docs: refine agent development guide`。每个 Commit 只覆盖一个明确的变更目的，不混入无关重构。
- **Branch 名称必须符合 `<type>/<kebab-case-description>`**，`type` 与主要修改类型一致；例如 `test/agent-server-callback-test`。禁止使用 Agent、模型或工具名称作为 branch prefix。
- **优先采用项目 Development Guide 定义的 change type vocabulary**。当前仓库没有 Development Guide 或已定义的类型词表，因此不要声称已有专属类型清单；在补充该指南前，分支 `type` 与对应的 Conventional Commits 类型保持一致。若后续新增 Development Guide，以其中的词表为准。
- 不要从少量现有提交推导固定的 Commit scope、合并策略或分支流程；分支操作与提交应遵守用户当前任务要求。
