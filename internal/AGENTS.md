# dan-tick Go 后端工作约定

本文件仅适用于 `internal/` 及其子目录，补充 [根目录指令](../AGENTS.md)。产品规则、跨层状态与安全约束、桌面调试和通用工作流遵守根文档。

实现前按任务阅读 [Wails 桌面后端 Pack](../docs/specs/packs/Wails%20桌面后端%20Pack.md) 及关联的架构、生命周期、调度和测试规范；项目取舍以 [技术栈选型](../docs/技术栈选型.md) 为准。此外按任务核对权威文档：持久化变更核对 [数据持久化设计](../docs/数据持久化设计.md)；调度、防重、错过与恢复行为变更核对 [PRD 第 7 节](../docs/PRD.md#7-调度与运行可靠性) 及 [计划游标与事务领取](../docs/数据持久化设计.md#3-计划游标与事务领取)；账号认证、扫码登录与续期变更核对 [B 站 API 对接参考的扫码登录与续期协议](../docs/B%20站%20API%20对接参考.md#13-扫码登录与续期)；构建与打包变更核对 [构建与发布](../docs/构建与发布.md)。

## 1. 技术栈与包职责

后端使用 **Bun + modernc.org/sqlite + Bun Migrations**、zerolog、`robfig/cron/v3`、`go-keyring`、`net/http` 和 `testing` / testify。按已有职责扩展具体类型，不引入 Gin、本地 REST Server、分布式调度或没有实际需求的 Repository 接口。

| 路径（相对本目录） | 职责 |
| --- | --- |
| `app/startup.go`、`app/desktop.go` | `Run(fs.FS)` 应用入口、诊断和启动失败处理；Wails、SQLite、Service、窗口、托盘及资源生命周期装配 |
| `app/runtime.go` | Keyring、共享 Sender / Executor / Scheduler、账号会话与配置 / 操作 Service 的业务组合根；Events 接线及启动 / 退出收尾 |
| `desktop/` | 配置、账号操作、执行历史、调度 / 总览、诊断与窗口 Wails Service；安全 DTO / 错误、窗口生命周期和托盘 |
| `account/`、`credential/` | Passport、扫码 / UID 绑定、受控续期、续期 / 发送互斥及单项 Keyring 整组会话 |
| `danmaku/`、`executor/` | B 站发送客户端、账号级共享发送器，以及单次执行的排队、停止、取消与记录 |
| `schedule/`、`scheduler/` | 时区 / 日历计划计算，以及 Cron 注册、持久化领取、防重、恢复和执行器交接 |
| `room/`、`task/` | 直播间与任务模型、校验和业务数据操作 |
| `storage/` | 用户数据目录中的 SQLite 连接、配置与 Bun Migration；迁移 SQL 位于 `storage/migrations/` |
| `history/` | 执行记录分页、详情、发送尝试与过期清理；不读取当前任务定义重建历史 |
| `settings/` | 现有 `app_settings` 中的非敏感配置；窗口关闭行为、默认时区与自动调度开关 |
| `diagnostic/` | 预定义安全诊断事件、日志轮换和安全诊断包导出；业务执行历史仍由 SQLite 保存 |

`app` 是应用组合根，其他包不得反向导入它。业务能力不依赖 Wails 窗口或 Binding；`desktop` 适配用例与平台能力，跨包协调由已有上层代码承担。保持单向依赖，不预建空包或无明确职责的 `utils` / `common`。

## 2. 模型、数据访问与事务

- Bun Model 默认允许业务复用，不为相同字段复制 Domain Model 与映射器。Wails 写入使用仅含可编辑字段的明确 Input，不直接接收含内部状态的完整数据库模型。
- 查询仅在敏感字段、计算 / 组合数据、序列化差异或公开契约独立演进等实际需求下创建 DTO。公开 Go 类型和 JSON 字段属于 Binding 契约，新增数据库字段不能自动暴露秘密或内部状态。
- 简单 CRUD 可由业务包直接使用注入的 `*bun.DB`；复用或复杂查询可放在所属包的具体方法 / 文件，不把所有 SQL 集中到 `storage`，不强制每表 Repository。
- 数据库使用用户数据目录中的 `dan-tick/dan-tick.db`，已配置 WAL、外键和单连接池。数据库 Migration 在依赖它的后台任务之前执行，失败则中止相关启动。
- 由编排业务用例的代码统一管理事务；防重与状态变更使用 SQLite 约束和原子写入，进程内锁不能替代。外部 HTTP、Cron 注册和 Wails Events 不参与 SQLite 事务，提交后的同步失败必须可报告和重新同步。
- 任务增删改、启停经共享 `scheduler.Manager` 在保存后同步 Cron；直播间删除后重建注册。不要绕过 Manager 直接改写运行中调度所依赖的配置。

## 3. Binding、错误与诊断

- Service 留在 `desktop/`，公开方法只暴露界面需要的操作和安全数据；Binding 负责输入输出适配和调用业务能力，不实现发送循环或平台凭证细节。
- 扫码登录的 Wails Binding 只返回临时二维码 PNG、登录阶段和候选 UID，不返回二维码 URL / key、Cookie 或 RT；用户离开页面即取消扫码，并忽略迟到响应。
- 调整公开 Service / 类型后检查生成 Binding 和前端导入契约；包扫描与生成物前置条件遵守根文档，不手写另一套通信接口。
- 可预期业务错误使用稳定类别，通过 `errors.Is` / `errors.As` 判断；内部错误用 `%w` 保留错误链，不按错误文字分支。
- `desktop/errors.go` 统一净化对外错误。Wails 会将 `error.Error()` 放入顶层消息，即使自定义 `MarshalError`，仍须同时保护顶层文字与 `cause` 分类；不直接暴露 SQLite 错误、第三方原始响应或堆栈。
- 诊断使用 `diagnostic` 预定义安全事件与字段。启动失败、Wails 错误和 panic 记录阶段 / 类别，保留现有脱敏行为，不输出原始错误或 panic 值；日志降级不能阻断正常业务。
- Events 在状态变化后通知前端刷新，不携带任务内容或凭证；持久化事实由业务存储负责，不能靠事件流恢复历史。

## 4. 应用与后台生命周期

- `app.Run` 接收根入口提供的前端 `fs.FS`，管理诊断和退出码；Wails 装配集中在 `app/desktop.go`，共享业务运行时仍由 `app/runtime.go` 管理。
- Wails 单实例所有权在打开 SQLite、恢复调度之前取得；第二次启动仅唤醒现有窗口。先绑定 Events，再启动调度；启动恢复完成后才注册 Cron。
- 所有发送路径复用同一 `danmaku.Sender` 和执行器，避免另建发送器绕过账号限流。账号限流与任务消息间隔按**发送结果返回后**计时，避免 Keyring 解锁、排队等待或 HTTP 耗时压缩实际请求间隔。调度与账号后台检查独立于前端挂载和窗口显示状态。
- 保留 `OnShutdown` 与 `defer` 收尾路径，并保证重复关闭安全；Wails 在部分平台可能不从 `Run` 返回。托盘查询、调度回调、账号检查、执行器和维护协程应按现有顺序停止并等待完成，再关闭 SQLite。
- 启动失败须关闭已构造资源，不留下半运行调度器。历史清理失败只记录安全诊断，不能阻止正常任务启动。
- 隐藏窗口、停止任务和退出进程保持各自语义；平台行为与人工验证遵守根文档，不以 Go 测试替代 Windows / Linux 桌面验收。

## 5. Go 验证

- 修改文件执行 `gofmt`，先运行受影响包测试，再按根文档准备构建产物并完成全仓 Go 检查。启动装配涉及 Wails 集成时执行 `wails3 build`；不要因为局部测试不导入根入口就省略全仓检查。
- 使用 `testing` / testify 验证公开行为、错误类别和数据约束；优先已有具体类型，仅在时钟、HTTP、Keyring 等真实外部边界需要替换时建立小接口，不为 Mock Bun 新增 Repository。
- 数据操作使用真实临时 SQLite，验证 Migration、外键、事务及防重；恢复测试关闭后重新打开数据库，不能只用进程内假数据。B 站请求用 `httptest` 验证业务响应、鉴权失败、超时、限流和结果不确定。
- 可控时间测试覆盖时区、夏令时、错过 / 恢复、状态变化、发送间隔、停止条件与取消。不要用真实长时间 `sleep` 证明正确性；调度库真实集成只做少量受控运行测试。
- 并发与生命周期变更运行 Race Detector（环境支持时），重点检查共享发送器、调度交接和关闭等待。迁移启动代码的测试可使用测试 `fs.FS`，不依赖真实前端产物来构造测试资源。
