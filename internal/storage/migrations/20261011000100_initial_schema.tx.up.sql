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

--bun:split

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

--bun:split

CREATE TABLE task_messages (
    id       INTEGER PRIMARY KEY,
    task_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    content  TEXT NOT NULL CHECK (length(trim(content)) > 0)
) STRICT;

--bun:split

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

--bun:split

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

--bun:split

CREATE TABLE app_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

--bun:split

CREATE INDEX idx_tasks_room ON tasks(room_id);

--bun:split

CREATE INDEX idx_tasks_due ON tasks(next_due_at)
    WHERE enabled = 1 AND deleted_at IS NULL AND next_due_at IS NOT NULL;

--bun:split

CREATE INDEX idx_task_messages_order ON task_messages(task_id, position, id);

--bun:split

CREATE UNIQUE INDEX uq_executions_scheduled ON executions(task_id, scheduled_at)
    WHERE trigger_kind = 'scheduled';

--bun:split

CREATE UNIQUE INDEX uq_executions_active_task ON executions(task_id)
    WHERE status IN ('waiting', 'queued', 'running');

--bun:split

CREATE INDEX idx_executions_task_recent ON executions(task_id, created_at DESC);

--bun:split

CREATE INDEX idx_executions_recent ON executions(created_at DESC);

--bun:split

CREATE INDEX idx_executions_active ON executions(status)
    WHERE status IN ('waiting', 'queued', 'running');
