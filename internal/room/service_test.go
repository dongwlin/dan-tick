package room

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

const testNow = int64(1_760_000_000_123)

func setupRoomService(t *testing.T, handler http.HandlerFunc) (*Service, *bun.DB) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "rooms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	service := NewService(db, testClient(server))
	service.now = func() time.Time { return time.UnixMilli(testNow) }
	return service, db
}

func validRoomResponse(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/room/v1/Room/get_info":
		id := r.URL.Query().Get("room_id")
		if id == "1" || id == "5440" {
			id = "5440"
		}
		fmt.Fprintf(w, `{"code":0,"data":{"room_id":%s,"uid":8,"title":"Room title"}}`, id)
	case "/live_user/v1/Master/info":
		fmt.Fprint(w, `{"code":0,"data":{"info":{"uname":"Anchor"}}}`)
	default:
		http.NotFound(w, r)
	}
}

func createTask(t *testing.T, db *bun.DB, roomID int64, name string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO tasks (room_id, name, kind, schedule_kind, local_time, next_due_at, created_at, updated_at)
		VALUES (?, ?, 'single', 'daily', '22:00:00', ?, ?, ?) RETURNING id`,
		roomID, name, testNow+86400000, testNow, testNow).Scan(&id)
	require.NoError(t, err)
	return id
}

func readTaskState(t *testing.T, db *bun.DB, id int64) (int, sql.NullInt64, sql.NullInt64) {
	t.Helper()
	var enabled int
	var due, deleted sql.NullInt64
	require.NoError(t, db.QueryRowContext(context.Background(), `
		SELECT enabled, next_due_at, deleted_at FROM tasks WHERE id = ?`, id).Scan(&enabled, &due, &deleted))
	return enabled, due, deleted
}

func TestRoomCRUDAndPersistence(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	ctx := context.Background()
	empty, err := s.List(ctx)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)

	first, err := s.Add(ctx, "https://live.bilibili.com/1")
	require.NoError(t, err)
	assert.Positive(t, first.ID)
	assert.Equal(t, "5440", first.PlatformRoomID)
	assert.Equal(t, "Room title", first.Title)
	assert.Equal(t, "Anchor", first.AnchorName)
	assert.Equal(t, testNow, first.CreatedAt)
	assert.Equal(t, testNow, first.UpdatedAt)
	assert.Nil(t, first.DeletedAt)
	assert.Zero(t, first.TaskCount)

	_, err = s.Add(ctx, "5440")
	require.ErrorIs(t, err, ErrAlreadyAdded)
	updated, err := s.UpdateRemark(ctx, first.ID, "  New remark  ")
	require.NoError(t, err)
	assert.Equal(t, "New remark", updated.Remark)
	taskID := createTask(t, db, first.ID, "daily")
	second, err := s.Add(ctx, "9")
	require.NoError(t, err)
	rooms, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 2)
	assert.Equal(t, second.ID, rooms[0].ID)
	assert.Equal(t, first.ID, rooms[1].ID)
	assert.Equal(t, 1, rooms[1].TaskCount)
	assert.Equal(t, "New remark", rooms[1].Remark)

	// A real close/reopen proves the records are not only in process memory.
	path := filepath.Join(t.TempDir(), "reopen.db")
	persistDB, err := storage.Open(ctx, path)
	require.NoError(t, err)
	ps := NewService(persistDB, s.client)
	ps.now = s.now
	persisted, err := ps.Add(ctx, "1")
	require.NoError(t, err)
	require.NoError(t, persistDB.Close())
	persistDB, err = storage.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, persistDB.Close()) })
	ps = NewService(persistDB, s.client)
	got, err := ps.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, persisted.ID, got[0].ID)
	assert.Equal(t, "5440", got[0].PlatformRoomID)

	assert.NoError(t, s.Delete(ctx, first.ID))
	enabled, due, deleted := readTaskState(t, db, taskID)
	assert.Zero(t, enabled)
	assert.False(t, due.Valid)
	assert.Equal(t, testNow, deleted.Int64)
	rooms, err = s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Equal(t, second.ID, rooms[0].ID)
}

func TestRestoreRoomKeepsIdentityRemarkAndHistoryButNotTasks(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	ctx := context.Background()
	original, err := s.Add(ctx, "1")
	require.NoError(t, err)
	_, err = s.UpdateRemark(ctx, original.ID, "My favorite")
	require.NoError(t, err)
	taskID := createTask(t, db, original.ID, "old task")
	_, err = db.ExecContext(ctx, `
		INSERT INTO executions (task_id, trigger_kind, status, created_at, finished_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'completed', ?, ?, 'old task', '5440')`, taskID, testNow, testNow)
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, original.ID))

	// A fresh short/long room lookup revives the room, not its deleted tasks.
	restored, err := s.Add(ctx, "https://live.bilibili.com/5440")
	require.NoError(t, err)
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.CreatedAt, restored.CreatedAt)
	assert.Equal(t, "My favorite", restored.Remark)
	assert.Nil(t, restored.DeletedAt)
	assert.Zero(t, restored.TaskCount)
	enabled, due, deleted := readTaskState(t, db, taskID)
	assert.Zero(t, enabled)
	assert.False(t, due.Valid)
	assert.True(t, deleted.Valid)
	var historyCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE task_id = ?`, taskID).Scan(&historyCount))
	assert.Equal(t, 1, historyCount)
	var storedCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rooms WHERE platform_room_id = '5440'`).Scan(&storedCount))
	assert.Equal(t, 1, storedCount)

	// A new task belongs to the restored room, without resurrecting old ones.
	createTask(t, db, restored.ID, "new task")
	rooms, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Equal(t, 1, rooms[0].TaskCount)
}

func TestRestorePreservesCachedDisplayInfoWhenOptionalFieldsMissing(t *testing.T) {
	var missing bool
	s, _ := setupRoomService(t, func(w http.ResponseWriter, r *http.Request) {
		if !missing {
			validRoomResponse(w, r)
			return
		}
		if r.URL.Path == "/room/v1/Room/get_info" {
			fmt.Fprint(w, `{"code":0,"data":{"room_id":5440,"uid":8}}`)
			return
		}
		http.Error(w, "unavailable", http.StatusBadGateway)
	})
	ctx := context.Background()
	before, err := s.Add(ctx, "1")
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, before.ID))
	missing = true
	after, err := s.Add(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, "Room title", after.Title)
	assert.Equal(t, "Anchor", after.AnchorName)
}

func TestConcurrentAddUsesUniqueCanonicalRoomID(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	inputs := []string{"1", "https://live.bilibili.com/5440"}
	var wg sync.WaitGroup
	results := make(chan error, len(inputs))
	for _, input := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Add(context.Background(), input)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, duplicate := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrAlreadyAdded):
			duplicate++
		default:
			t.Fatalf("unexpected concurrent add error: %v", err)
		}
	}
	assert.Equal(t, 1, success)
	assert.Equal(t, 1, duplicate)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM rooms`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRoomOperationsRejectDeletedOrUnknownIDs(t *testing.T) {
	s, _ := setupRoomService(t, validRoomResponse)
	ctx := context.Background()
	for _, id := range []int64{-1, 0, 42} {
		_, err := s.UpdateRemark(ctx, id, "remark")
		require.ErrorIs(t, err, ErrNotSaved)
		require.ErrorIs(t, s.Delete(ctx, id), ErrNotSaved)
	}
	room, err := s.Add(ctx, "1")
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, room.ID))
	_, err = s.UpdateRemark(ctx, room.ID, "not allowed")
	require.ErrorIs(t, err, ErrNotSaved)
	require.ErrorIs(t, s.Delete(ctx, room.ID), ErrNotSaved)
}

func TestDeleteRoomWithActiveExecutionRollsBack(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	ctx := context.Background()
	room, err := s.Add(ctx, "1")
	require.NoError(t, err)
	taskID := createTask(t, db, room.ID, "active task")
	_, err = db.ExecContext(ctx, `
		INSERT INTO executions (task_id, trigger_kind, status, created_at, task_name_snapshot, platform_room_id_snapshot)
		VALUES (?, 'manual', 'running', ?, 'active task', '5440')`, taskID, testNow)
	require.NoError(t, err)
	require.ErrorIs(t, s.Delete(ctx, room.ID), ErrRoomBusy)
	rooms, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Nil(t, rooms[0].DeletedAt)
	enabled, due, deleted := readTaskState(t, db, taskID)
	assert.Equal(t, 1, enabled)
	assert.True(t, due.Valid)
	assert.False(t, deleted.Valid)
	_, err = db.ExecContext(ctx, `UPDATE executions SET status = 'stopped', finished_at = ? WHERE task_id = ?`, testNow, taskID)
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, room.ID))
}

func TestRoomDeleteDBFailureRollsBack(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	ctx := context.Background()
	room, err := s.Add(ctx, "1")
	require.NoError(t, err)
	taskID := createTask(t, db, room.ID, "prevent deletion")
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER reject_task_deletion BEFORE UPDATE OF deleted_at ON tasks
		BEGIN SELECT RAISE(ABORT, 'task update failed'); END`)
	require.NoError(t, err)
	require.ErrorContains(t, s.Delete(ctx, room.ID), "task update failed")
	rooms, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Nil(t, rooms[0].DeletedAt)
	enabled, due, deleted := readTaskState(t, db, taskID)
	assert.Equal(t, 1, enabled)
	assert.True(t, due.Valid)
	assert.False(t, deleted.Valid)
}

func TestRoomMetadataIsOptionalAndReadIsOffline(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if r.URL.Path == "/room/v1/Room/get_info" {
			fmt.Fprint(w, `{"code":0,"data":{"room_id":5440,"uid":8}}`)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}
	s, db := setupRoomService(t, handler)
	ctx := context.Background()
	room, err := s.Add(ctx, "1")
	require.NoError(t, err)
	assert.Empty(t, room.Title)
	assert.Empty(t, room.AnchorName)
	taskID := createTask(t, db, room.ID, "still scheduled")
	mu.Lock()
	count := requests
	mu.Unlock()
	for range 2 {
		rooms, err := s.List(ctx)
		require.NoError(t, err)
		require.Len(t, rooms, 1)
		assert.Equal(t, 1, rooms[0].TaskCount)
	}
	mu.Lock()
	assert.Equal(t, count, requests, "local listing must not depend on metadata network calls")
	mu.Unlock()
	enabled, due, deleted := readTaskState(t, db, taskID)
	assert.Equal(t, 1, enabled)
	assert.True(t, due.Valid)
	assert.False(t, deleted.Valid)
}

func TestRoomAddFailureDoesNotWrite(t *testing.T) {
	s, db := setupRoomService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := s.Add(context.Background(), "1")
	require.ErrorIs(t, err, ErrUnavailable)
	_, err = s.Add(context.Background(), "https://example.com/1")
	require.ErrorIs(t, err, ErrInvalidInput)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM rooms`).Scan(&count))
	assert.Zero(t, count)
}

func TestRoomAddHonorsCancelledContext(t *testing.T) {
	s, db := setupRoomService(t, validRoomResponse)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Add(ctx, "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM rooms`).Scan(&count))
	assert.Zero(t, count)
}

func TestRoomAddPersistsVerifiedRoomWhenOptionalMetadataTimesOut(t *testing.T) {
	s, db := setupRoomService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/room/v1/Room/get_info" {
			fmt.Fprint(w, `{"code":0,"data":{"room_id":5440,"uid":8,"title":"Room title"}}`)
			return
		}
		<-r.Context().Done()
	})
	s.client.timeout = 100 * time.Millisecond
	ctx := context.Background()
	saved, err := s.Add(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, "5440", saved.PlatformRoomID)
	assert.Equal(t, "Room title", saved.Title)
	assert.Empty(t, saved.AnchorName)
	rooms, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Equal(t, saved, rooms[0])
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rooms`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestRoomAddDoesNotPersistWhenCallerCancelsOptionalMetadata(t *testing.T) {
	masterStarted := make(chan struct{})
	s, db := setupRoomService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/room/v1/Room/get_info" {
			fmt.Fprint(w, `{"code":0,"data":{"room_id":5440,"uid":8,"title":"Room title"}}`)
			return
		}
		close(masterStarted)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-masterStarted
		cancel()
	}()
	_, err := s.Add(ctx, "1")
	require.ErrorIs(t, err, context.Canceled)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM rooms`).Scan(&count))
	assert.Zero(t, count)
}
