package storage

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestOpenReopenAndConstraints(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "my data", "dan tick.db")
	db, err := Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	for _, table := range []string{
		"rooms", "tasks", "task_messages", "executions", "send_attempts", "app_settings",
	} {
		var sqlText string
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&sqlText))
		require.Contains(t, sqlText, "STRICT")
	}
	for _, index := range []string{
		"idx_tasks_room", "idx_tasks_due", "idx_task_messages_order", "uq_executions_scheduled",
		"uq_executions_active_task", "idx_executions_task_recent", "idx_executions_recent", "idx_executions_active",
	} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", index).Scan(&count))
		require.Equal(t, 1, count, index)
	}

	var fk, busyTimeout int
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk))
	require.Equal(t, 1, fk)
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout))
	require.Equal(t, 5000, busyTimeout)
	var mode string
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
	require.Equal(t, "wal", mode)

	// STRICT types, uniqueness and foreign keys are enforced by the real driver.
	_, err = db.ExecContext(ctx, "INSERT INTO rooms (platform_room_id, created_at, updated_at) VALUES ('42', 'wrong type', 1000)")
	require.Error(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO tasks (room_id, name, kind, schedule_kind, local_time, created_at, updated_at) VALUES (999, 'task', 'single', 'daily', '12:00:00', 1000, 1000)")
	require.Error(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO rooms (id, platform_room_id, created_at, updated_at) VALUES (1, '42', 1000, 1000)")
	require.NoError(t, err)
	var cover string
	var checked int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT cover_url, metadata_checked_at FROM rooms WHERE id = 1").Scan(&cover, &checked))
	require.Empty(t, cover)
	require.Zero(t, checked, "legacy rooms must be stale for the first metadata refresh")
	_, err = db.ExecContext(ctx, "INSERT INTO rooms (platform_room_id, created_at, updated_at) VALUES ('42', 1000, 1000)")
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tasks
		(id, room_id, name, kind, schedule_kind, local_time, next_due_at, created_at, updated_at)
		VALUES (1, 1, 'daily task', 'single', 'daily', '12:00:00', 1791710400000, 1000, 1000)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO task_messages (task_id, position, content) VALUES (1, 0, 'hello')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM rooms WHERE id = 1")
	require.Error(t, err, "a room referenced by a task must be protected")

	// A scheduled execution has a unique plan identity; active executions cannot overlap.
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, scheduled_at, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (1, 'scheduled', 1791710400000, 'waiting', 1000, 'daily task', '42')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, scheduled_at, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (1, 'scheduled', 1791710400000, 'missed', 1001, 'daily task', '42')`)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (1, 'manual', 'queued', 1001, 'daily task', '42')`)
	require.Error(t, err)
	// Test sends are room-scoped executions with no owning task.
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (NULL, 'test', 'queued', 1002, '测试发送', '42')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (1, 'test', 'queued', 1003, '测试发送', '42')`)
	require.Error(t, err, "a test send must not reference a task")
	_, err = db.ExecContext(ctx, `INSERT INTO executions
		(task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (NULL, 'manual', 'queued', 1004, 'daily task', '42')`)
	require.Error(t, err, "manual executions must reference a task")

	_, err = db.ExecContext(ctx, "INSERT INTO app_settings (key, value) VALUES ('timezone', 'Asia/Shanghai')")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Opening again runs only unapplied migrations and preserves the user data.
	db, err = Open(ctx, path)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM bun_migrations").Scan(&count))
	require.Equal(t, 3, count)
	var due int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT next_due_at FROM tasks WHERE id = 1").Scan(&due))
	require.Equal(t, int64(1791710400000), due)
	var setting string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key = 'timezone'").Scan(&setting))
	require.Equal(t, "Asia/Shanghai", setting)
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk))
	require.Equal(t, 1, fk)
	_, err = db.ExecContext(ctx, "DELETE FROM executions WHERE task_id = 1")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM tasks WHERE id = 1")
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM task_messages").Scan(&count))
	require.Zero(t, count, "task messages must cascade on physical task deletion")
}

func TestFailedMigrationRollsBackAndCanRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "failed.db")
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := connectSQLite(ctx, uri.String())
	require.NoError(t, err)

	broken := fstest.MapFS{
		"20261011000100_bad.tx.up.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE partial_table (id INTEGER PRIMARY KEY) STRICT;
--bun:split
CREATE TABLE another_table (id INTEGER PRIMARY KEY) STRICT;
--bun:split
THIS IS NOT SQL;
`)},
	}
	require.Error(t, runMigrations(ctx, db, broken))
	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('partial_table', 'another_table')").Scan(&count))
	require.Zero(t, count, "transactional DDL must roll back on failure")
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM bun_migrations").Scan(&count))
	require.Zero(t, count, "failed migrations must not be marked applied")
	require.NoError(t, db.Close())

	// The same database file remains usable for a subsequent successful migration.
	db, err = Open(ctx, path)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM bun_migrations").Scan(&count))
	require.Equal(t, 3, count)
}

func TestUpgradeExistingRoomsKeepsUserDataAndMarksMetadataStale(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "existing.db")
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := connectSQLite(ctx, uri.String())
	require.NoError(t, err)
	initial, err := migrationFiles.ReadFile("migrations/20261011000100_initial_schema.tx.up.sql")
	require.NoError(t, err)
	legacy := fstest.MapFS{
		"20261011000100_initial_schema.tx.up.sql": &fstest.MapFile{Data: initial},
	}
	require.NoError(t, runMigrations(ctx, db, legacy))
	_, err = db.ExecContext(ctx, `
		INSERT INTO rooms (platform_room_id, title, anchor_name, remark, created_at, updated_at)
		VALUES ('5440', 'Saved title', 'Saved anchor', 'User remark', 1000, 1000)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = Open(ctx, path)
	require.NoError(t, err)
	defer db.Close()
	var title, anchor, remark, cover string
	var checked int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT title, anchor_name, remark, cover_url, metadata_checked_at
		FROM rooms WHERE platform_room_id = '5440'`).Scan(&title, &anchor, &remark, &cover, &checked))
	require.Equal(t, "Saved title", title)
	require.Equal(t, "Saved anchor", anchor)
	require.Equal(t, "User remark", remark)
	require.Empty(t, cover)
	require.Zero(t, checked)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM bun_migrations").Scan(&count))
	require.Equal(t, 3, count)
}

func TestOpenDoesNotOverwriteCorruptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	contents := []byte("existing user database contents")
	require.NoError(t, os.WriteFile(path, contents, 0600))
	_, err := Open(context.Background(), path)
	require.Error(t, err)
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, contents, actual)
}

func TestUpgradeExecutionHistoryForTestSendsPreservesRowsAndAllowsNullTask(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade-test-sends.db")
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := connectSQLite(ctx, uri.String())
	require.NoError(t, err)

	initial, err := migrationFiles.ReadFile("migrations/20261011000100_initial_schema.tx.up.sql")
	require.NoError(t, err)
	metadata, err := migrationFiles.ReadFile("migrations/20261012000100_room_metadata.tx.up.sql")
	require.NoError(t, err)
	legacy := fstest.MapFS{
		"20261011000100_initial_schema.tx.up.sql": &fstest.MapFile{Data: initial},
		"20261012000100_room_metadata.tx.up.sql":  &fstest.MapFile{Data: metadata},
	}
	require.NoError(t, runMigrations(ctx, db, legacy))
	_, err = db.ExecContext(ctx, `INSERT INTO rooms (id, platform_room_id, created_at, updated_at)
VALUES (1, '42', 1, 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tasks (id, room_id, name, kind, schedule_kind,
local_time, created_at, updated_at) VALUES (1, 1, 'task', 'single', 'daily', '12:00:00', 1, 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO executions (id, task_id, trigger_kind, status,
created_at, finished_at, sent_count, failed_count, task_name_snapshot,
platform_room_id_snapshot) VALUES (7, 1, 'manual', 'completed', 1, 2, 1, 0, 'task', '42')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO send_attempts (id, execution_id, sequence, message_no,
content_snapshot, attempted_at, result) VALUES (9, 7, 1, 1, 'old message', 1, 'accepted')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	var taskID int64
	var status string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT e.task_id, e.status
FROM executions e JOIN send_attempts a ON a.execution_id = e.id
WHERE e.id = 7 AND a.id = 9`).Scan(&taskID, &status))
	require.EqualValues(t, 1, taskID)
	require.Equal(t, "completed", status)

	_, err = db.ExecContext(ctx, `INSERT INTO executions (task_id, trigger_kind, status,
created_at, finished_at, error_code, task_name_snapshot, platform_room_id_snapshot)
VALUES (NULL, 'test', 'completed', 3, 4, NULL, '测试发送', '42')`)
	require.NoError(t, err, "upgraded schema must accept test sends without a task")

	var fkViolations int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&fkViolations))
	require.Zero(t, fkViolations)
}
