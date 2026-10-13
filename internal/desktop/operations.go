package desktop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dongwlin/dan-tick/internal/account"
	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/executor"
	"github.com/dongwlin/dan-tick/internal/room"
	"github.com/uptrace/bun"
)

const (
	EventAccountChanged   = "dan-tick:account-changed"
	EventExecutionChanged = "dan-tick:execution-changed"
)

type credentialsStore interface {
	Clear() error
}
type qrLoginService interface {
	BeginQRLogin(context.Context) (account.QRLoginStatus, error)
	PollQRLogin(context.Context) (account.QRLoginStatus, error)
	ConfirmQRAccount(context.Context) error
	CancelQRLogin(context.Context) error
}
type loginChecker interface {
	Check(context.Context) (account.Status, error)
}
type testRunner interface {
	TestSend(context.Context, string, string) (executor.ManualResult, error)
}
type manualRunner interface {
	StartManual(int64) (int64, error)
	StopExecution(int64) error
}

// OperationsService is a Wails boundary. No method returns Cookie data,
// underlying OS/keyring errors, message bodies or upstream response strings.
type OperationsService struct {
	db      *bun.DB
	store   credentialsStore
	account loginChecker
	tester  testRunner
	runner  manualRunner
	emit    func(string)
}

func NewOperationsService(db *bun.DB, store credentialsStore, checker loginChecker, tester testRunner, runner manualRunner, emit func(string)) *OperationsService {
	return &OperationsService{db: db, store: store, account: checker, tester: tester, runner: runner, emit: emit}
}

func (s *OperationsService) changed(event string) {
	if s.emit != nil {
		s.emit(event)
	}
}

func (s *OperationsService) CheckAccount(ctx context.Context) account.Status {
	status, err := s.account.Check(ctx)
	if errors.Is(err, credential.ErrReauthenticationRequired) {
		return account.Status{State: account.ReauthenticationRequired}
	}
	if err != nil {
		return account.Status{State: account.VerificationFailed}
	}
	return status
}

func (s *OperationsService) BeginQRLogin(ctx context.Context) (account.QRLoginStatus, error) {
	owner, ok := s.account.(qrLoginService)
	if !ok {
		return account.QRLoginStatus{}, protect(account.ErrSessionClosed)
	}
	result, err := owner.BeginQRLogin(ctx)
	return result, protect(err)
}

func (s *OperationsService) PollQRLogin(ctx context.Context) (account.QRLoginStatus, error) {
	owner, ok := s.account.(qrLoginService)
	if !ok {
		return account.QRLoginStatus{}, protect(account.ErrSessionClosed)
	}
	result, err := owner.PollQRLogin(ctx)
	return result, protect(err)
}

func (s *OperationsService) ConfirmQRAccount(ctx context.Context) error {
	owner, ok := s.account.(qrLoginService)
	if !ok {
		return protect(account.ErrSessionClosed)
	}
	return protect(owner.ConfirmQRAccount(ctx))
}

func (s *OperationsService) CancelQRLogin(ctx context.Context) error {
	owner, ok := s.account.(qrLoginService)
	if !ok {
		return protect(account.ErrSessionClosed)
	}
	return protect(owner.CancelQRLogin(ctx))
}

func (s *OperationsService) ClearAccount(_ context.Context) error {
	if err := s.store.Clear(); err != nil {
		return protect(err)
	}
	s.changed(EventAccountChanged)
	return nil
}

type TestSendInput struct {
	RoomID  int64  `json:"roomId"`
	Message string `json:"message"`
}

type TestSendResult struct {
	Outcome   danmaku.Outcome `json:"outcome"`
	ErrorCode string          `json:"errorCode"`
}

// TestSend uses the same FIFO account limiter and execution pipeline as
// scheduled and manual runs. Every accepted test send creates an execution
// row and a pending attempt; expected send failures are returned as safe
// result categories, not arbitrary upstream error strings.
func (s *OperationsService) TestSend(ctx context.Context, input TestSendInput) (TestSendResult, error) {
	if input.RoomID <= 0 {
		return TestSendResult{}, protect(danmaku.ErrInvalidInput)
	}
	if err := danmaku.ValidateMessage(input.Message); err != nil {
		return TestSendResult{}, protect(err)
	}
	var platformID string
	err := s.db.QueryRowContext(ctx, `SELECT platform_room_id FROM rooms WHERE id = ? AND deleted_at IS NULL`, input.RoomID).Scan(&platformID)
	if errors.Is(err, sql.ErrNoRows) {
		return TestSendResult{}, protect(room.ErrNotSaved)
	}
	if err != nil {
		return TestSendResult{}, protect(err)
	}
	result, err := s.tester.TestSend(ctx, platformID, input.Message)
	if err != nil {
		return TestSendResult{}, protect(err)
	}
	return TestSendResult{Outcome: result.Outcome, ErrorCode: result.ErrorCode}, nil
}

type StartManualResult struct {
	ExecutionID int64 `json:"executionId"`
}

func (s *OperationsService) StartManual(_ context.Context, taskID int64) (StartManualResult, error) {
	id, err := s.runner.StartManual(taskID)
	if err != nil {
		return StartManualResult{}, protect(err)
	}
	return StartManualResult{ExecutionID: id}, nil
}

func (s *OperationsService) StopExecution(_ context.Context, executionID int64) error {
	return protect(s.runner.StopExecution(executionID))
}

type ActiveExecution struct {
	ID          int64  `json:"id"`
	TaskID      int64  `json:"taskId"`
	Status      string `json:"status"`
	SentCount   int64  `json:"sentCount"`
	FailedCount int64  `json:"failedCount"`
}

// LatestExecutions reads a fresh authoritative SQLite snapshot; the Event
// payload is only an invalidation hint, never a second state source.
func (s *OperationsService) LatestExecutions(ctx context.Context) ([]ActiveExecution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, task_id, status, sent_count, failed_count
		FROM executions e WHERE id = (SELECT MAX(id) FROM executions WHERE task_id = e.task_id)
		ORDER BY id DESC`)
	if err != nil {
		return nil, protect(err)
	}
	defer rows.Close()
	items := make([]ActiveExecution, 0)
	for rows.Next() {
		var item ActiveExecution
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Status, &item.SentCount, &item.FailedCount); err != nil {
			return nil, protect(fmt.Errorf("read active execution: %w", err))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, protect(err)
	}
	return items, nil
}
