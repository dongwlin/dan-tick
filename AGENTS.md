# 项目协作指南

## 项目定位

- 本项目是面向 Windows 和 Linux 的 B 站直播弹幕定时发送桌面应用，使用 Wails 3 和 Go，采用本地优先的方式运行。
- 开始实现功能前先阅读 [`docs/PRD.md`](docs/PRD.md)。需求范围、业务规则和验收标准以该文档为依据；需求未明确的技术选择不要自行视为已定案。
- 技术选型与桌面适配例外见 [`docs/技术栈选型.md`](docs/技术栈选型.md)。工程规范位于 [`docs/specs/`](docs/specs/)：前端从 [Vue SPA Pack](docs/specs/packs/Vue%20SPA%20Pack.md) 和 [亚克力设计 Pack](docs/specs/packs/亚克力设计%20Pack.md) 开始，后端从 [Wails 桌面后端 Pack](docs/specs/packs/Wails%20桌面后端%20Pack.md) 开始；按任务阅读相关文档，项目选型与通用规范冲突时以技术栈选型文档为准。
- 当前代码仍以 Wails 模板为主，示例问候服务和页面不是目标产品的已实现功能。不要把 PRD 中的规划描述成现有能力。

## 当前技术栈与目录

- `main.go`、`greetservice.go`：Go 后端及 Wails 应用入口，目前主要是模板示例。
- `frontend/src/`：Vue 3 + TypeScript 前端，使用 Composition API 和 `<script setup>`。
- `frontend/vite.config.ts`、`frontend/package.json`：Vite 配置与前端脚本。
- `Taskfile.yml`、`build/`：Wails 3 开发、构建和平台打包配置。
- `docs/PRD.md`：MVP 产品需求文档。根目录与前端的 `README.md` 目前是模板说明，不应代替 PRD。
- `docs/技术栈选型.md`、`docs/specs/`：项目技术决策及从通用规范仓库引入的前后端规范。上游副本的来源与更新记录见 `docs/specs/spec-pack-manifest.json`。

## 实现约定

- 保持实现简单、清晰、易维护；按现有结构渐进演进，避免为尚未确定的功能提前引入框架、抽象和依赖。
- 调度、时区计算、任务状态持久化、HTTP 发送与频率控制由 Go 后端负责；前端负责配置、展示和交互，不承担后台任务执行。
- 后端采用 Bun + 纯 Go SQLite 驱动、zerolog、`robfig/cron/v3` 和 testify。Cron 负责计划触发；数据库防重、错过不补发与外部发送副作用由业务层保障，不能只依赖 Cron 包装器。
- 优先保证任务不会重复执行、错过调度不被意外补发、状态可恢复。具体行为遵循 PRD，尤其是调度、后台运行与异常恢复要求。
- B 站凭证属于敏感信息：不得写入日志、提交到仓库或以明文写入普通配置；优先使用操作系统安全存储，不得静默降级。认证请求由后端处理。
- 前端采用 Vue 3 + TypeScript、Tailwind CSS v4、Reka UI、Lucide 与亚克力设计系统；Pinia 负责共享客户端状态、TanStack Query 缓存 Go 业务查询，通过 Wails Binding / Events 同步，不重复保存同一业务状态。按需引入依赖，具体约束以技术栈选型文档为准。
- 不手动修改构建产物或 Wails 自动生成的绑定文件；需要时通过项目构建流程重新生成。
- 修改应聚焦当前任务。修复问题时优先解决根因，测试覆盖关键行为和真实边界，不添加与需求无关的复杂防御或重复测试。

## 开发与验证

- 在仓库根目录运行 `wails3 dev` 启动开发环境，运行 `wails3 build` 构建桌面应用。
- 在 `frontend/` 运行 `npm run build` 可执行前端类型检查与构建（需要先安装前端依赖）。
- Go 代码使用 `gofmt` 格式化；涉及后端逻辑时按需执行 `go test ./...`。注意 `main.go` 会嵌入 `frontend/dist`，独立运行 Go 命令前可能需要先构建前端。
- 优先验证受影响范围；若因环境依赖或生成文件缺失而无法运行检查，应如实说明，不能将未运行的检查描述为通过。
- 保留用户已有的未提交改动；未经明确要求，不主动提交、重置或清理工作区。
