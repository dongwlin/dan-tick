# dan-tick

dan-tick 是一款面向 Windows 和 Linux 的 B 站直播弹幕定时发送桌面应用。计划支持本地管理直播间、弹幕任务、定时调度与执行记录，由 Go 后端直接调用 B 站接口，不依赖打开直播间网页。

**当前处于开发阶段，尚未发布正式版本。** 正式 Wails 3 应用已接入业务界面、扫码账号、调度、发送与执行记录；Windows / Linux 实机验收仍待完成。`prototype/` 是独立 UI 原型，使用演示数据，不会调用真实 B 站接口。

## 预览 UI 原型

需要 Node.js 和 pnpm：

```sh
cd prototype
pnpm install
pnpm dev
```

按终端提示打开本地地址（默认 `http://localhost:5173`）。原型包含仪表盘、弹幕任务、直播间、执行记录、设置和任务编辑等页面，支持演示交互；刷新后演示数据会恢复初始状态。具体说明见 [原型 README](prototype/README.md)。

## 运行正式应用

需要 Go 1.25、Node.js 24.18.0、pnpm 12.6.0、Wails 3 CLI，以及目标平台所需的 Wails 构建依赖。正式前端的 Node 版本记录在 `frontend/.node-version`，pnpm 版本由 `frontend/package.json` 的 `packageManager` 固定。

```sh
cd frontend
pnpm install --frozen-lockfile
cd ..
wails3 dev
```

构建当前平台的桌面应用：

```sh
wails3 build
```

桌面构建产物位于 `bin/`。正式前端与 `prototype/` 均使用 pnpm，但分别管理依赖与锁文件，不合并为 workspace。正式前端的单独构建和依赖升级说明见 [前端 README](frontend/README.md)。

CI 自动构建 Linux amd64 AppImage、Windows amd64 EXE 和用户级 NSIS 安装包，普通构建上传 Actions artifacts，只有推送正式版本 Tag 才发布 Release。版本由 Git 自动生成，无需每次修改配置；命名、配置同步、下载和发布步骤见 [构建与发布](docs/构建与发布.md)。

Linux 采用**薄 AppImage + 系统 WebKit**：包内仅包含应用二进制、启动入口和图标，运行前需安装发行版的 GTK4、WebKitGTK 6.0 与 GIO TLS 运行时，安装命令见 [Linux 运行依赖](docs/构建与发布.md#linux-appimage-运行依赖与兼容范围)。发布以 Ubuntu 24.04 为编译基线，适配目标为 Ubuntu 26.04、Debian 13 和 Fedora 44；Ubuntu 24.04 及其他旧版本仅尽力兼容。CI 在安装系统运行时的三个目标发行版容器中检查同一最终包的真实页面、Binding 和沙箱状态，正常桌面验收仍待完成。

## 仓库结构

| 位置 | 用途 |
| --- | --- |
| `main.go`、`internal/app/` | 资源嵌入入口、应用装配与生命周期 |
| `internal/storage/` | Bun + SQLite 连接、事务型数据库迁移及集成测试 |
| `internal/diagnostic/` | 结构化安全诊断日志、轮转与保留策略 |
| `frontend/` | 正式 Wails 应用的 Vue 3 + TypeScript 前端 |
| `prototype/` | 独立 UI 演示原型，Vanilla TypeScript + Vite |
| `build/`、`Taskfile.yml` | Wails 开发、构建和平台打包配置 |
| `docs/` | 产品需求、技术决策、开发计划和数据库设计文档 |

## 诊断日志

正式应用启动时默认记录本地 JSON Lines 诊断日志，数据库初始化失败也会记录。活动文件为 `dan-tick.jsonl`；单文件最多 10 MiB，总共最多 5 个文件（含活动文件），轮转日志最多保留 14 天。日志与 SQLite 执行历史、Keyring 凭证相互独立。

- Linux：通常为 `~/.local/state/dan-tick/logs/`（可受 `XDG_STATE_HOME` 影响）。
- Windows：通常为 `%LOCALAPPDATA%\dan-tick\logs\`。

设置页提供「打开日志目录」和主动导出经过敏感信息检查的诊断包。日志不可写时不会阻止应用运行，界面可读取降级状态；日志与诊断导出的实机可操作性仍需对应平台验收。

## 文档入口

- [产品需求（PRD）](docs/PRD.md)：功能范围、优先级和验收标准。
- [开发计划](docs/开发计划.md)：开发顺序、最小可提交单元及各阶段完成条件。
- [Roadmap](docs/ROADMAP.md)：以 v0.0.1 为首个 MVP 发布版本的版本规划与长期方向。
- [技术栈选型](docs/技术栈选型.md)：已确定的技术决策、前后端职责和规范例外。
- [构建与发布](docs/构建与发布.md)：CI 产物、Tag 自动版本、平台打包和 Release 流程。
- [后端数据模型设计](docs/后端数据模型设计.md)及 [SQLite Schema 与 Migration 设计](docs/SQLite%20Schema%20与%20Migration%20设计.md)：业务实体、已落地的首版表结构与后续事务设计。
- [B 站 API 对接参考](docs/B%20站%20API%20对接参考.md)：待实际接入时验证的第三方接口信息。
- [UI 原型说明](prototype/README.md)：演示功能、运行命令与源码结构。
- [项目开发约定](AGENTS.md)与 [引入的工程规范](docs/specs/packs/)：实现时的架构和编码要求；`docs/specs/` 为外部规范复制件。
