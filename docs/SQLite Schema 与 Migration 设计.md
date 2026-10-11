# SQLite Schema 与 Migration 设计

## 1. 范围

本文细化 [后端数据模型设计](后端数据模型设计.md) 的 MVP 落地规则。六张业务表、索引及外键已由 `internal/storage/migrations/20261011000100_initial_schema.tx.up.sql` 首次落地；应用在启动时通过 `internal/storage` 打开并迁移数据库。任务创建、编辑、启用 / 禁用、逻辑删除与消息持久化已经落地，执行与恢复仍待后续单元实现。

保持六张业务表：`rooms`、`tasks`、`task_messages`、`executions`、`send_attempts`、`app_settings`。B3 / B4 已实现任务与消息的创建、读取、编辑、状态变更及逻辑删除；执行与恢复等业务事务仍待后续单元实现。不为单账号创建账号表，不为每个任务单独创建计划表，也不引入 Repository 框架、状态事件表或常驻任务队列表。

## 2. SQLite 类型与通用约定

| 数据 | SQLite 类型 | 约定 |
| --- | --- | --- |
| 内部 ID / 外键 | `INTEGER` | 主键使用 `INTEGER PRIMARY KEY`；不额外使用 `AUTOINCREMENT` |
| B 站直播间 ID | `TEXT` | 入库前解析为规范化十进制字符串，保留与本地主键的区分；实际调用接口前解析 / 验证 |
| UTC 时刻 | `INTEGER` | Unix 毫秒时间戳；`created_at`、`next_due_at`、`scheduled_at` 等全部统一 |
| 本地日期 / 时间 | `TEXT` | `local_date` 使用 `YYYY-MM-DD`，`local_time` 使用 `HH:MM:SS`；解释时始终结合任务 `timezone` |
| 时区 | `TEXT` | IANA 名称，默认 `Asia/Shanghai`，由 Go 的 `time.LoadLocation` 校验 |
| 星期集合 | `INTEGER` | 7 位位图：周一 bit 0，……，周日 bit 6，范围 1–127 |
| 布尔值 | `INTEGER` | 仅允许 0 / 1，写入 Go `bool` |
| 枚举 / 文本 | `TEXT` | 数据库存稳定的英文值；用户可见名称由前端翻译 |
| 可选值 | `NULL` | 未适用 / 尚未发生使用 `NULL`，不拿 0 或空字符串冒充时间 |

所有业务表使用 SQLite `STRICT` 表，避免类型亲和性导致意外写入；当前选定的 `modernc.org/sqlite` 支持此语法，正式引入依赖时以实际版本验证。Go 写入时间统一转换 UTC 毫秒；Bun Model 使用明确的整数毫秒字段或显式转换，不依赖驱动自动解析 `time.Time`。精度不要求纳秒；不得将文本时间与整数时间混存。

下列 SQL 对应**已落地的首个 Migration**；后续以版本化 SQL 文件为准。只在数据库层放稳定、低成本的约束；跨表的业务校验、内容长度限制、Cron 计算与状态迁移由 Go 负责。

## 3. 目标建表 SQL

### 3.1 直播间与任务

```sql
CREATE TABLE rooms (
    id               INTEGER PRIMARY KEY,
    platform_room_id TEXT NOT NULL UNIQUE CHECK (length(platform_room_id) > 0),
    title            TEXT NOT NULL DEFAULT '',
    anchor_name      TEXT NOT NULL DEFAULT '',
    remark           TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    deleted_at       INTEGER
) STRICT;

CREATE TABLE tasks (
    id                   INTEGER PRIMARY KEY,
    room_id              INTEGER NOT NULL REFERENCES rooms(id) ON DELETE RESTRICT,
    name                 TEXT NOT NULL CHECK (length(trim(name)) > 0),
    kind                 TEXT NOT NULL CHECK (kind IN ('single', 'loop')),
    enabled              INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    interval_seconds     INTEGER CHECK (interval_seconds IS NULL OR interval_seconds > 0),
    max_count            INTEGER CHECK (max_count IS NULL OR max_count > 0),
    max_duration_seconds INTEGER CHECK (max_duration_seconds IS NULL OR max_duration_seconds > 0),
    schedule_kind        TEXT NOT NULL CHECK (schedule_kind IN ('once', 'daily', 'weekly')),
    timezone             TEXT NOT NULL DEFAULT 'Asia/Shanghai',
    local_date           TEXT,
    local_time           TEXT NOT NULL,
    weekdays             INTEGER,
    next_due_at          INTEGER,
    created_at           INTEGER NOT NULL,
    updated_at           INTEGER NOT NULL,
    deleted_at           INTEGER,
    CHECK (
        (kind = 'single' AND interval_seconds IS NULL
            AND max_count IS NULL AND max_duration_seconds IS NULL)
        OR
        (kind = 'loop' AND interval_seconds IS NOT NULL
            AND (max_count IS NOT NULL OR max_duration_seconds IS NOT NULL))
    ),
    CHECK (
        (schedule_kind = 'once' AND local_date IS NOT NULL AND weekdays IS NULL)
        OR (schedule_kind = 'daily' AND local_date IS NULL AND weekdays IS NULL)
        OR (schedule_kind = 'weekly' AND local_date IS NULL
            AND weekdays BETWEEN 1 AND 127)
    )
) STRICT;

CREATE TABLE task_messages (
    id       INTEGER PRIMARY KEY,
    task_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    content  TEXT NOT NULL CHECK (length(trim(content)) > 0)
) STRICT;
```

- `single` 恰好一条弹幕；`loop` 至少一条弹幕，并设置间隔及至少一种停止条件。弹幕条数属于跨行规则，由保存任务的 Go 事务负责；不是 SQL `CHECK` 能可靠表达的条件。
- `interval_seconds` 只有循环任务使用；`max_count` 以循环中的**逻辑消息次数**计，有限 HTTP 重试不会重复推进内容游标，重试仍必须遵守全局限流。
- `next_due_at` 保存下一次**未处理**计划时刻，不是最新 Cron 回调时间。禁用、删除、已结束或错过的一次性计划令其为 `NULL`；手动执行不修改它。重新启用前由业务计算未来时刻。
- `platform_room_id` 全表唯一（包括逻辑删除记录）。重新添加同一直播间时恢复原记录，避免同一个外部直播间产生多个本地身份。
- `position` 可重复；读取以 `position, id` 稳定排序，调整顺序时不需要绕过唯一约束。

### 3.2 执行与发送尝试

```sql
CREATE TABLE executions (
    id                        INTEGER PRIMARY KEY,
    task_id                   INTEGER NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    trigger_kind              TEXT NOT NULL CHECK (trigger_kind IN ('scheduled', 'manual')),
    scheduled_at              INTEGER,
    status                    TEXT NOT NULL CHECK (status IN (
        'waiting', 'queued', 'running', 'completed',
        'stopped', 'failed', 'missed', 'interrupted'
    )),
    created_at                INTEGER NOT NULL,
    queued_at                 INTEGER,
    started_at                INTEGER,
    finished_at               INTEGER,
    sent_count                INTEGER NOT NULL DEFAULT 0 CHECK (sent_count >= 0),
    failed_count              INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    error_code                TEXT,
    error_message             TEXT,
    task_name_snapshot        TEXT NOT NULL,
    platform_room_id_snapshot TEXT NOT NULL,
    CHECK (
        (trigger_kind = 'scheduled' AND scheduled_at IS NOT NULL)
        OR (trigger_kind = 'manual' AND scheduled_at IS NULL)
    ),
    CHECK (status != 'missed' OR trigger_kind = 'scheduled')
) STRICT;

CREATE TABLE send_attempts (
    id               INTEGER PRIMARY KEY,
    execution_id     INTEGER NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    sequence         INTEGER NOT NULL CHECK (sequence > 0),
    message_no       INTEGER NOT NULL CHECK (message_no > 0),
    content_snapshot TEXT NOT NULL,
    attempted_at     INTEGER NOT NULL,
    result           TEXT NOT NULL CHECK (result IN ('pending', 'accepted', 'failed', 'unknown')),
    error_code       TEXT,
    UNIQUE (execution_id, sequence)
) STRICT;

CREATE TABLE app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;
```

- `waiting` 是已登记、待交给执行器的状态；`queued` 表示等待账号级发送能力；`running` 表示已开始本次执行。其余状态均为终态，`missed` 仅用于计划执行。队列不是独立的持久化表。
- `queued_at`、`started_at`、`finished_at` 分别记录进入队列、开始实际执行、结束时刻；未发生的阶段保持 `NULL`。历史列表按 `created_at` 排序，不依赖可能为空的 `started_at`。
- 每个终态执行都必须写入 `finished_at`，包括 `missed` 和 `interrupted`。30 天历史清理只删除 `finished_at` 早于截止时间的终态执行，不删除仍在运行的实例。
- `task_name_snapshot` 和 `platform_room_id_snapshot` 保证任务改名或逻辑删除后历史可辨认；直播间筛选仍可沿 `executions → tasks → rooms` 关联，不复制整份任务配置。
- `sequence` 是一次执行内每次 HTTP 请求尝试的递增序号；`message_no` 是本次执行内逻辑消息序号，重试同一条消息时不变。因此可以辨认有限重试而无需额外的重试关系表。
- 每次请求**发送前**先提交一条 `pending` 记录，收到确定答复后更新为 `accepted` 或 `failed`；超时或结果无法判定时标为 `unknown`。崩溃恢复时遗留 `pending` 一律视为结果未知，**不自动重新发送**。业务响应成功只代表接口接受请求。
- `sent_count` 统计确认被服务端接受的逻辑消息数；`failed_count` 统计最终未确认成功的逻辑消息数（包括未知结果），不把一条消息的每次失败重试重复计数。具体错误类别写 `error_code`，可展示的 `error_message` 必须先脱敏，不能保存 Cookie 或完整请求头。
- “立即执行任务”使用 `manual` 执行实例，不占用计划唯一键、不推进 `next_due_at`。独立输入的“测试发送”只返回本次结果，首版不创建任务执行记录，仍共享账号级限流。
- `app_settings` 只存普通文本设置，可由 Go 按键解释其值；不存登录凭证，也不为每个设置创建一列。

## 4. 外键、唯一约束与索引

```sql
CREATE INDEX idx_tasks_room ON tasks(room_id);
CREATE INDEX idx_tasks_due ON tasks(next_due_at)
    WHERE enabled = 1 AND deleted_at IS NULL AND next_due_at IS NOT NULL;
CREATE INDEX idx_task_messages_order ON task_messages(task_id, position, id);

CREATE UNIQUE INDEX uq_executions_scheduled ON executions(task_id, scheduled_at)
    WHERE trigger_kind = 'scheduled';
CREATE UNIQUE INDEX uq_executions_active_task ON executions(task_id)
    WHERE status IN ('waiting', 'queued', 'running');
CREATE INDEX idx_executions_task_recent ON executions(task_id, created_at DESC);
CREATE INDEX idx_executions_recent ON executions(created_at DESC);
CREATE INDEX idx_executions_active ON executions(status)
    WHERE status IN ('waiting', 'queued', 'running');
```

主键、`rooms.platform_room_id` 以及 `send_attempts(execution_id, sequence)` 的唯一索引由表约束隐式提供，不重复创建。执行尝试按 `execution_id, sequence` 读取可复用对应唯一索引。索引主要服务于直播间任务列表、下一次待执行扫描、弹幕排序、历史列表和崩溃恢复；其他筛选索引等有实际查询和 `EXPLAIN QUERY PLAN` 证据后再加。

两条执行部分唯一索引承担不同职责：

1. `uq_executions_scheduled`：同一任务同一个 **UTC 计划时刻**最多登记一次计划实例；即便已完成、失败或错过也不能再次登记。手动执行不受约束。
2. `uq_executions_active_task`：同一任务在任意时刻只允许一个活跃执行（包括手动）。新手动执行在冲突时返回“任务正在执行”；新的计划实例在冲突时登记为 `missed`，并记录原因、推进游标，而不是启动第二个循环。

数据库外键明确采用：`tasks → rooms` 和 `executions → tasks` 为 `RESTRICT`，保护被历史引用的实体；`task_messages → tasks` 与 `send_attempts → executions` 为 `CASCADE`，方便对应实体**最终物理清理**时移除子记录。正常用户删除只做逻辑删除：直播间删除时同步禁用并逻辑删除关联任务，停止 / 处理活跃执行，撤销未来调度；历史仍可查询。过期历史先物理删除 `executions`（带走 `send_attempts`）；确需清理已逻辑删除任务或直播间时，待其不再被历史引用后按子到父的顺序删除。

## 5. 事务与防重

SQLite 只允许一个写事务同时提交。首版采用一个 Bun / `database/sql` 连接池，优先设置 `SetMaxOpenConns(1)` 简化本地写入协调；**进程内串行不替代持久化唯一约束和事务**。事务必须短，不包围外部 HTTP、Cron 注册、Events 或等待限流。

| 场景 | 必须在同一事务内完成 |
| --- | --- |
| 新建 / 编辑任务 | 校验关联直播间未删除；更新任务字段、完整弹幕列表和计算后的 `next_due_at` |
| 禁用 / 逻辑删除任务 | 更新启用 / 删除标记并清除未来游标；活跃执行按用户选择在事务之外发起取消，再持久化最终状态 |
| 自动计划登记 | 校验任务 / 直播间仍有效、启用且 `next_due_at` 等于待领取计划时刻；判定过期 / 冲突；写入执行记录并推进下一游标 |
| 发送状态 | 写入请求前的 `pending` 尝试；请求结束后更新尝试结果和执行汇总，不发生“成功结果已保存但计数未更新” |
| 历史清理 | 删除到期执行记录及其尝试；不改变任务定义和调度游标 |

**自动计划领取顺序：**

1. 以数据库中读出的 `tasks.next_due_at` 作为 `scheduled_at`，而不是以 Cron 回调实际到达时的 `time.Now()` 作为计划身份。进入事务后重新核验任务版本 / 游标、启用状态与直播间有效性。
2. 对比真实 UTC 当前时间和项目允许的迟到窗口；已过期记 `missed`，存在同任务活跃执行也记 `missed`，其余创建 `waiting`。`missed` 包含 `finished_at` 和清晰原因。
3. 用带 `WHERE id = ? AND next_due_at = ? AND enabled = 1 AND deleted_at IS NULL` 的**条件更新**领取计划，并推进下一次待处理游标；受影响行数为 0 则该回调已失效。唯一约束冲突不能误当新的可执行实例，也不能清理别的事务的结果；需要区分“已登记”和“活跃任务冲突”，必要时把活跃冲突改写为 `missed`。同一事务提交后才能把新建的 `waiting` 实例交给执行器。
4. 如果事务已提交、进程却在交给执行器前崩溃，下次启动将 `waiting` 标为 `interrupted`，**不会在不确定是否发生外部副作用的情况下自动重放**。Cron 与 Wails Event 更新发生在提交后，失败时从 SQLite 重建，不假装数据库回滚。

仅检查内存锁、只查询“最近执行”或依赖 Cron 的 `SkipIfStillRunning` 都无法保证此语义。手动执行在独立事务内检查任务与活跃执行并插入 `manual` 实例，完全不触碰计划游标。

应用启动 / 恢复时，先将遗留的 `waiting`、`queued`、`running` 执行标为 `interrupted`，将仍为 `pending` 的尝试标为 `unknown` 并核对计数；之后再恢复调度。对长期离线的周期任务，最多为每次恢复窗口保留一条代表性的 `missed` 记录，然后直接计算下一次未来时刻，不逐次补发或批量生成历史；一次性任务则终结为 `missed` 并清空游标。持久化游标不会因为系统时间倒拨而自动后退；编辑计划时也不自动创建过去的计划实例。

本阶段不固定迟到宽限秒数，该阈值在调度实现前与测试一起确定。时区中的本地时间不存在时跳过该天；重复出现时只使用**第一次**真实时刻。每日 / 每周计划从当前计划所属的本地日历日向后计算，避免同一天第二个重复时刻再次入队；唯一键仍以 UTC `scheduled_at` 防重。这些规则需要在调度实现中验证 `robfig/cron/v3` 的计算结果，并用明确日历边界测试固定下来。

## 6. Migration 与数据库初始化

数据库初始化已按以下方案落地；异常恢复与调度等后续阶段仍按开发计划实现：

1. `internal/storage` 负责打开数据库、配置连接、运行迁移和关闭资源。数据库位于系统用户数据目录；失败时停止依赖数据库的调度和 Wails Service 初始化，不自动删除或重建用户数据库。
2. SQL 迁移文件放在 `internal/storage/migrations/`，按时间戳和目的命名，例如 `20261010000100_initial_schema.tx.up.sql` / `20261010000100_initial_schema.tx.down.sql`。使用 Bun `migrate.NewMigrations`、`Discover`、`Migrator.Init` 和 `Migrator.Migrate`（或实际版本提供的等价 API），在编译期通过 `go:embed` 内嵌 SQL。多条 SQL 用 `--bun:split` 分隔；采用 `.tx.up.sql`，让一组 Schema DDL 在一个事务内执行。不要在应用运行时使用 `NewCreateTable` / 自动同步 Model 来代替版本迁移。
3. 数据库连接打开后、开始事务前启用并验证 `PRAGMA foreign_keys = ON`；**每个物理连接都必须启用**，可通过 `modernc.org/sqlite` 的连接 DSN `_pragma=foreign_keys(1)` 设置。配置有界的 `busy_timeout`（例如 5 秒），按本地文件数据库需要启用 WAL；不要在迁移事务内部切换 `journal_mode` 或 `foreign_keys`。不提前调整大量 SQLite 参数。
4. 先迁移、再执行异常恢复与游标校对、最后启动 Cron / 执行器并注册 Wails Service。Bun 自己维护迁移版本元数据；不重复创建自定义 `schema_version` 表。
5. 已发布迁移不可直接修改；后续 Schema 修改追加新迁移。遇到不兼容结构改动时先备份、再使用 SQLite 支持的表重建方式迁移，验证数据和 `PRAGMA foreign_key_check`，不能静默丢弃记录。回滚 SQL 优先服务开发环境；正式升级失败须报告并停止启动，不自动向下迁移或覆盖用户文件。运行中数据库备份必须采用 SQLite 一致性快照机制，不直接复制仍在写入的 WAL 主文件。

初始 `.tx.down.sql` 只需按外键依赖反序删除上述六张业务表及对应索引；不删除 Bun 自己的元数据表。正式版本不向普通用户暴露“回滚并清空数据”操作。

实现时参考 [Bun Migrations](https://bun.uptrace.dev/guide/migrations.html)、[modernc SQLite 驱动说明](https://pkg.go.dev/modernc.org/sqlite) 和 [SQLite PRAGMA 文档](https://www.sqlite.org/pragma.html)，以引入的依赖版本和实际集成测试结果为准。

## 7. 落地验证的最小清单

- 从空文件迁移、重复启动、重新打开文件后字段及外键仍有效；迁移中故意失败时不留下半建成的业务 Schema。
- `STRICT` 类型、时区 / 整数毫秒往返、直播间唯一恢复、任务完整保存与回滚。
- 同一 `(task_id, scheduled_at)` 重复触发只登记一次；两个不同计划时刻不能同时运行同一个任务；手动执行不推进计划。
- 重启后处理遗留活跃实例与 `pending` 尝试，不产生额外 HTTP；大量错过的计划只保留代表性记录。
- 逻辑删除后的历史关联、30 天历史清理及执行尝试级联删除；外键在实际数据库连接上被执行。

通过真实的临时 SQLite + Bun 集成测试验证上述风险，不为每个 `CHECK` 或每个字段建立重复测试。基础迁移已有 `internal/storage/storage_test.go` 覆盖；其余业务一致性与恢复场景由后续开发单元实现。
