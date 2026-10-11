# dan-tick

dan-tick 是一款面向 Windows 和 Linux 的 B 站直播弹幕定时发送桌面应用。计划支持本地管理直播间、弹幕任务、定时调度与执行记录，由 Go 后端直接调用 B 站接口，不依赖打开直播间网页。

**当前处于开发阶段，尚不能实际发送弹幕。** 仓库中的正式 Wails 3 应用仍以模板为主；`prototype/` 是可交互的独立 UI 原型，使用演示数据，不会调用真实 B 站接口。原型不等于已实现的桌面业务功能。

## 预览 UI 原型

需要 Node.js 和 pnpm：

```sh
cd prototype
pnpm install
pnpm dev
```

按终端提示打开本地地址（默认 `http://localhost:5173`）。原型包含仪表盘、弹幕任务、直播间、执行记录、设置和任务编辑等页面，支持演示交互；刷新后演示数据会恢复初始状态。具体说明见 [原型 README](prototype/README.md)。

## 运行正式应用模板

需要 Go 1.25、Node.js / npm、Wails 3 CLI，以及目标平台所需的 Wails 构建依赖。

```sh
cd frontend
npm install
cd ..
wails3 dev
```

构建当前平台的桌面应用：

```sh
wails3 build
```

桌面构建产物位于 `bin/`。当前 `frontend/` 仍为 Vue 模板页面，以上命令不会启动 UI 原型，也不会提供尚未实现的弹幕功能。正式前端默认使用 npm；`prototype/` 独立使用 pnpm。

## 仓库结构

| 位置 | 用途 |
| --- | --- |
| `main.go`、`greetservice.go` | 当前 Wails 模板入口与示例服务 |
| `frontend/` | 正式 Wails 应用的 Vue 3 + TypeScript 前端（尚未实现业务页面） |
| `prototype/` | 独立 UI 演示原型，Vanilla TypeScript + Vite |
| `build/`、`Taskfile.yml` | Wails 开发、构建和平台打包配置 |
| `docs/` | 产品需求、技术决策和实现前的设计文档 |

## 文档入口

- [产品需求（PRD）](docs/PRD.md)：功能范围、优先级和验收标准。
- [开发计划](docs/开发计划.md)：开发顺序、最小可提交单元及各阶段完成条件。
- [技术栈选型](docs/技术栈选型.md)：已确定的技术决策、前后端职责和规范例外。
- [后端数据模型设计](docs/后端数据模型设计.md)及 [SQLite Schema 与 Migration 设计](docs/SQLite%20Schema%20与%20Migration%20设计.md)：业务实体与数据库设计草案，尚未落地。
- [B 站 API 对接参考](docs/B%20站%20API%20对接参考.md)：待实际接入时验证的第三方接口信息。
- [UI 原型说明](prototype/README.md)：演示功能、运行命令与源码结构。
- [项目开发约定](AGENTS.md)与 [引入的工程规范](docs/specs/packs/)：实现时的架构和编码要求；`docs/specs/` 为外部规范复制件。
