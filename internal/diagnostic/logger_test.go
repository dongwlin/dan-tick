package diagnostic

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoggerPersistsSafeEventsAcrossRestarts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	first := New(dir)
	require.False(t, first.Status().Degraded)
	first.Record(AppStarting)
	first.Record(DatabaseOpenFailed)
	first.Close()

	second := New(dir)
	require.False(t, second.Status().Degraded)
	second.Record(AppStopped)
	second.Close()

	f, err := os.Open(filepath.Join(dir, activeName))
	require.NoError(t, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var events []string
	for scanner.Scan() {
		var record map[string]any
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		require.NotEmpty(t, record["time"])
		require.Contains(t, []string{"info", "error"}, record["level"])
		require.NotContains(t, record, "message")
		require.NotContains(t, record, "error")
		require.NotContains(t, record, "cookie")
		events = append(events, record["event"].(string))
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, []string{"app.starting", "database.open_failed", "app.stopped"}, events)
	info, err := os.Stat(filepath.Join(dir, activeName))
	require.NoError(t, err)
	if os.PathSeparator != '\\' {
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestLoggerConcurrentWritesProduceWholeLines(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	const workers, perWorker = 12, 60
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				l.Record(DatabaseReady)
			}
		}()
	}
	wg.Wait()
	l.Close()
	data, err := os.ReadFile(filepath.Join(dir, activeName))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, workers*perWorker)
	for _, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "database.ready", record["event"])
	}
}

func TestExecutionEventFieldsAreRestricted(t *testing.T) {
	dir := t.TempDir()
	logger := New(dir)
	logger.RecordExecution(ExecutionStarted, 8, 12, "")
	logger.RecordExecution(SendUnknown, 8, 12, "cookie=private session")
	logger.RecordExecution(ExecutionFailed, 8, 12, "timeout")
	logger.RecordExecution(AppStarting, 8, 12, "credential_missing")
	logger.RecordExecution(SendAccepted, 0, 12, "")
	logger.RecordExecution(ExecutionQueued, 8, 12, "")
	logger.RecordExecution(ExecutionConflict, 8, 12, "cookie=private session")
	logger.RecordSendAttempt(SendAccepted, 8, 12, 2, "")
	logger.RecordSendAttempt(SendRejected, 8, 12, -1, "rate_limited")
	logger.RecordSendAttempt(AppFailed, 8, 12, 3, "send_failure")
	logger.Close()

	data, err := os.ReadFile(filepath.Join(dir, activeName))
	require.NoError(t, err)
	require.NotContains(t, string(data), "private")
	require.NotContains(t, string(data), "cookie")
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 6)
	for i, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.EqualValues(t, 8, record["execution_id"])
		require.EqualValues(t, 12, record["task_id"])
		require.NotContains(t, record, "message")
		if i == 1 {
			require.Equal(t, "unknown", record["result"])
			require.Equal(t, "send_failure", record["error_category"])
		}
		if i == 2 {
			require.Equal(t, "timeout", record["error_category"])
		}
		if i == 5 {
			require.EqualValues(t, 2, record["attempt_sequence"])
			require.Equal(t, "accepted", record["result"])
		}
		if i == 4 {
			require.Equal(t, "execution.conflict", record["event"])
			require.Equal(t, "task_busy", record["error_category"])
		}
	}
}

func TestScheduledEventRequiresKnownTypeAndValidIdentifiers(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	l.RecordSchedule(AppStarting, 1, 2, 3)
	l.RecordSchedule(ScheduleClaimed, 0, 2, 3)
	l.RecordSchedule(ScheduleConflict, 1, -2, 3)
	l.RecordSchedule(ScheduleClaimed, 1, 2, 0)
	l.RecordSchedule(ScheduleClaimed, 3, 4, 1760000000000)
	l.Close()
	data, err := os.ReadFile(filepath.Join(dir, activeName))
	require.NoError(t, err)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record))
	require.Equal(t, "schedule.claimed", record["event"])
	require.EqualValues(t, 3, record["execution_id"])
	require.EqualValues(t, 4, record["task_id"])
	require.EqualValues(t, 1760000000000, record["scheduled_at"])
	require.NotContains(t, record, "error")
	require.NotContains(t, record, "message")
	require.Len(t, strings.Split(strings.TrimSpace(string(data)), "\n"), 1)
}

func TestRollingWriterEnforcesSizeAndFileCount(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	w := newRollingWriter(dir, policy{maxSize: 100, maxFiles: 3, maxAge: 14 * 24 * time.Hour}, func() time.Time { return now })
	for range 30 {
		_, err := w.Write([]byte(strings.Repeat("x", 60) + "\n"))
		require.NoError(t, err)
	}
	require.False(t, w.status().Degraded)
	w.close()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 3) // active file and 2 backups
	var backups int
	for _, e := range entries {
		info, err := e.Info()
		require.NoError(t, err)
		require.LessOrEqual(t, info.Size(), int64(100))
		if backupName.MatchString(e.Name()) {
			backups++
		}
	}
	require.Equal(t, 2, backups)
}

func TestRollingWriterExpiresOldFilesWithoutDeletingOtherFiles(t *testing.T) {
	dir := t.TempDir()
	old := "dan-tick-20260901T030000.000000000.jsonl"
	other := "other-app-20260901T030000.000000000.jsonl"
	require.NoError(t, os.WriteFile(filepath.Join(dir, old), []byte("old"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, other), []byte("keep"), 0600))
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	w := newRollingWriter(dir, policy{maxSize: 100, maxFiles: 5, maxAge: 14 * 24 * time.Hour}, func() time.Time { return now })
	_, err := os.Stat(filepath.Join(dir, old))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, other))
	require.NoError(t, err)

	// A quiet but long-running application rotates on the next calendar day,
	// allowing the old active file to age out even without size pressure.
	_, err = w.Write([]byte("yesterday\n"))
	require.NoError(t, err)
	now = now.Add(16 * 24 * time.Hour)
	_, err = w.Write([]byte("today\n"))
	require.NoError(t, err)
	w.close()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2) // other app file and the current active log
	data, err := os.ReadFile(filepath.Join(dir, activeName))
	require.NoError(t, err)
	require.Equal(t, "today\n", string(data))
}

func TestLoggerDegradesWithoutPreventingWritesOrStartup(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "not-a-directory")
	require.NoError(t, os.WriteFile(path, []byte("do not replace"), 0600))
	l := New(path)
	require.Equal(t, Status{Directory: path, Degraded: true, Warning: warningText}, l.Status())
	l.Record(AppStarting)
	l.Record(DatabaseOpenFailed)
	l.Close()
	require.FileExists(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "do not replace", string(data))
}

func TestWriterFailureDegradesAndCloseIsIdempotent(t *testing.T) {
	w := newRollingWriter(t.TempDir(), defaultPolicy(), time.Now)
	require.NoError(t, w.file.Close()) // simulate a failed underlying write
	_, err := w.Write([]byte("{}\n"))
	require.NoError(t, err)
	require.True(t, w.status().Degraded)
	w.close()
	w.close()
	_, err = w.Write([]byte("{}\n"))
	require.NoError(t, err)
}
