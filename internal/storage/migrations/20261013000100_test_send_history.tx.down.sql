-- Test-send rows cannot satisfy the previous task_id NOT NULL shape, so a
-- downgrade discards only those rows (attempts cascade) before rebuilding.

DELETE FROM executions WHERE trigger_kind = 'test';

--bun:split

CREATE TABLE executions_previous (
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

INSERT INTO executions_previous (
    id, task_id, trigger_kind, scheduled_at, status, created_at, queued_at,
    started_at, finished_at, sent_count, failed_count, error_code, error_message,
    task_name_snapshot, platform_room_id_snapshot
)
SELECT
    id, task_id, trigger_kind, scheduled_at, status, created_at, queued_at,
    started_at, finished_at, sent_count, failed_count, error_code, error_message,
    task_name_snapshot, platform_room_id_snapshot
FROM executions;

--bun:split

CREATE TABLE send_attempts_previous (
    id               INTEGER PRIMARY KEY,
    execution_id     INTEGER NOT NULL REFERENCES executions_previous(id) ON DELETE CASCADE,
    sequence         INTEGER NOT NULL CHECK (sequence > 0),
    message_no       INTEGER NOT NULL CHECK (message_no > 0),
    content_snapshot TEXT NOT NULL,
    attempted_at     INTEGER NOT NULL,
    result           TEXT NOT NULL CHECK (result IN ('pending', 'accepted', 'failed', 'unknown')),
    error_code       TEXT,
    UNIQUE (execution_id, sequence)
) STRICT;

--bun:split

INSERT INTO send_attempts_previous (
    id, execution_id, sequence, message_no, content_snapshot, attempted_at, result, error_code
)
SELECT id, execution_id, sequence, message_no, content_snapshot, attempted_at, result, error_code
FROM send_attempts;

--bun:split

DROP TABLE send_attempts;

--bun:split

DROP TABLE executions;

--bun:split

ALTER TABLE executions_previous RENAME TO executions;

--bun:split

ALTER TABLE send_attempts_previous RENAME TO send_attempts;

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
