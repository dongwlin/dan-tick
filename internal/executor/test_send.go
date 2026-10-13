package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/uptrace/bun"
)

// TestSendTaskName is the persisted task-name snapshot for test sends. Test
// sends have no owning task, so the value is a stable label rather than a
// mutable task name copied at execution time.
const TestSendTaskName = "测试发送"

// TestSend performs one real test send through the same account-wide Sender as
// scheduled and manual runs, and persists it in the shared execution timeline
// as trigger_kind = 'test'. A known send failure is returned as a terminal
// ManualResult instead of an error, matching RunManual semantics; only
// registration or persistence failures produce an error.
func (s *Service) TestSend(ctx context.Context, roomID, message string) (ManualResult, error) {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(message) == "" {
		return ManualResult{}, danmaku.ErrInvalidInput
	}
	reg, err := s.registerTestSend(ctx, roomID, message)
	if err != nil {
		return ManualResult{}, err
	}
	return s.finishTestSend(reg)
}

// registerTestSend mirrors manual registration without a task: it commits the
// queued execution and its pending attempt atomically before any HTTP request.
func (s *Service) registerTestSend(ctx context.Context, roomID, message string) (manualRegistration, error) {
	started := s.now()
	runCtx, stop := context.WithCancel(ctx)
	run := &activeRun{cancel: stop, testSend: true}
	var executionID int64

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		stop()
		return manualRegistration{}, ErrExecutorClosed
	}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := started.UTC().UnixMilli()
		if err := tx.QueryRowContext(ctx, `
INSERT INTO executions (task_id, trigger_kind, status, created_at,
queued_at, task_name_snapshot, platform_room_id_snapshot)
VALUES (NULL, 'test', 'queued', ?, ?, ?, ?) RETURNING id`,
			now, now, TestSendTaskName, roomID).Scan(&executionID); err != nil {
			return fmt.Errorf("register test execution: %w", err)
		}
		return insertPending(ctx, tx, executionID, 1, message, now)
	})
	if err == nil {
		if s.active == nil {
			s.active = make(map[int64]*activeRun)
		}
		s.active[executionID] = run
		s.done.Add(1)
	}
	s.mu.Unlock()
	if err != nil {
		stop()
		return manualRegistration{}, err
	}
	s.record(diagnostic.ExecutionQueued, executionID, 0, "")
	return manualRegistration{
		ctx: runCtx, cancel: stop, run: run,
		plan:    runPlan{roomID: roomID, messages: []string{message}},
		started: started, id: executionID, taskID: 0,
	}, nil
}

func (s *Service) finishTestSend(reg manualRegistration) (ManualResult, error) {
	defer reg.cancel()
	defer func() {
		s.mu.Lock()
		delete(s.active, reg.id)
		s.mu.Unlock()
		s.done.Done()
	}()
	return s.execute(reg.ctx, reg.run, reg.id, 0, reg.plan, reg.started)
}
