# Go 桌面应用技术栈

## 1. 适用范围

本 Profile 定义 Wails 3 + Go 本地优先桌面应用的后端默认技术选型。面向 Windows / Linux 单进程应用，不定义 HTTP 服务端或云数据库架构；具体业务模型、调度语义和平台支持范围由项目决定。

## 2. 默认选型

| 领域 | 默认选择 | 说明 |
| --- | --- | --- |
| 桌面框架 | Wails 3 | Service Binding、Events 与桌面生命周期 |
| 语言 | Go | 使用项目 `go.mod` 声明的版本 |
| 本地数据库 | SQLite | 嵌入式持久化，不要求独立服务 |
| SQLite 驱动 | `modernc.org/sqlite` | 纯 Go `database/sql` 驱动；不通过自动切换驱动的 shim 选择实现 |
| ORM | `github.com/uptrace/bun` + `sqlitedialect` | SQL-first 查询、模型映射与事务 |
| 数据迁移 | Bun Migrations | 使用版本化迁移；复杂度允许时直接维护 SQL |
| 日志 | `github.com/rs/zerolog` | 结构化诊断日志 |
| 调度 | `github.com/robfig/cron/v3` | 周期任务触发和下一次执行时间计算 |
| 凭证 | `github.com/zalando/go-keyring` | Windows Credential Manager、Linux Secret Service |
| 外部 HTTP | `net/http` | 显式超时、取消、响应解析 |
| 测试 | `testing` + `github.com/stretchr/testify` | 真实 SQLite 测试，`assert` / `require` |

依赖版本由项目锁定；只有出现实际需求时才引入其他框架、ORM、日志系统或缓存。

## 3. SQLite 与数据访问

- 使用 `database/sql` 打开 `modernc.org/sqlite` 注册的 `sqlite` 驱动，以 Bun SQLite Dialect 处理查询和事务；不同时维护另一套常规 SQL 数据访问框架。
- 数据库保存在操作系统推荐的用户数据目录，不放在安装目录或进程工作目录。
- 数据迁移在依赖它的后台任务启动前完成；迁移失败必须中止依赖数据库的服务启动，禁止忽略错误继续执行。
- SQLite 外键约束、写入并发策略和事务边界必须显式配置或验证；涉及任务执行登记、唯一约束和状态更改时使用数据库事务或等效原子操作。
- 需要查询时优先使用 Bun 已有功能，不为每张表机械创建 Repository 接口。持久化模型与对外 Service 参数只在存在真实边界差异时分离。

## 4. 依赖与业务边界

- 通过构造函数显式组装实际存在的长生命周期对象（数据库、调度器、HTTP Client、凭证存储）；优先具体类型，不引入依赖注入框架。
- 业务规则不放在 Wails Binding 方法、Bun Hook 或数据库迁移中。Binding 适配输入输出，业务 Service 负责用例编排。
- 调度器只负责计划触发，执行器负责单次任务生命周期，第三方 HTTP Client 负责请求；不让调度回调直接实现网络发送规则。
- 对真正可替换的外部边界（例如时钟、HTTP 请求与凭证服务）可以使用小接口；禁止为了 Mock 每个 Bun 查询而制造接口层。

## 5. 日志、配置与凭证

- zerolog 用于应用诊断日志，业务执行记录写入 SQLite；两者不互相替代。
- 日志内容遵循最小必要原则；不得输出 Cookie、令牌、完整认证请求头或其他可复用凭证。错误消息在跨越 Binding 前必须移除敏感细节。
- 非敏感设置使用应用配置或 SQLite 持久化；不因为配置量少就引入 Viper。
- 凭证通过操作系统 Keyring 保存，普通 SQLite 与配置文件不得保存明文。Keyring 不可用或被锁定时必须明确告知用户，不自动降级为明文存储。
- 对外 HTTP Client 必须明确超时、取消和错误分类；无法确定副作用是否发生的写请求默认不自动重试。

## 6. 非默认方案

Gin、HTTP Handler / Middleware、PostgreSQL、Bun PostgreSQL 方言、PASETO、Viper、Huma 和通用 Cron 表达式编辑器均不是本 Profile 的默认组成部分。确有对应产品需求时再独立选型。
