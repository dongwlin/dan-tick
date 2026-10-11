package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/robfig/cron/v3"
	"github.com/uptrace/bun"
)

var (
	ErrScheduleSync    = errors.New("task saved but schedule synchronization failed")
	ErrRecoveryNeeded  = errors.New("overdue schedule requires recovery")
	ErrSchedulerClosed = errors.New("scheduler is stopped")
	ErrNoHandoff       = errors.New("scheduled execution handoff is required")
)

// Manager coordinates Cron wakeups with the persisted task cursor. A job is a
// one-shot wakeup for exactly one SQLite occurrence; after a successful claim
// the next persisted occurrence is registered. Using A3's calendar calculation
// rather than an independent Cron expression preserves DST gap/fold semantics.
//
// onReady receives ONLY committed waiting executions. Its implementation is
// the future scheduled executor; no HTTP request is made by this package.
// onError reports failures from asynchronous callbacks, including handoff
// errors. A failed handoff is not retried because the waiting row is durable.
type Manager struct {
	db      *bun.DB
	tasks   *task.Service
	claim   *ClaimService
	cron    *cron.Cron
	onReady func(context.Context, ClaimResult) error
	onError func(error)
	log     *diagnostic.Logger
	now     func() time.Time

	mu         sync.Mutex
	entries    map[int64]registration
	generation uint64
	started    bool
	closed     bool
	lastError  error
	ctx        context.Context
	cancel     context.CancelFunc
}

type registration struct {
	id         cron.EntryID
	due        int64
	generation uint64
}

type pendingPlan struct {
	id  int64
	due int64
}

// NewManager requires a real handoff function before Start can run. Until an
// E-stage scheduled executor is available, no production scheduler is started.
func NewManager(db *bun.DB, log *diagnostic.Logger,
	onReady func(context.Context, ClaimResult) error, onError func(error)) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{db: db, tasks: task.NewService(db), claim: NewClaimService(db, log),
		cron: cron.New(cron.WithLocation(time.UTC)), onReady: onReady, onError: onError,
		log: log, now: time.Now, ctx: ctx, cancel: cancel,
		entries: make(map[int64]registration)}
}

// Start reconstitutes all registrations from SQLite before enabling callbacks.
// E3 must reconcile overdue cursors first; starting with any overdue task is
// an explicit error, not implicit permission to backfill it.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrSchedulerClosed
	}
	if m.onReady == nil {
		return ErrNoHandoff
	}
	if m.started {
		return nil
	}
	if err := m.rebuildLocked(ctx); err != nil {
		return err
	}
	m.cron.Start()
	m.started = true
	return nil
}

// Stop prevents new registrations/callbacks and cancels callback contexts.
// The returned context is done once Cron callbacks have exited. An already
// handed-off execution remains the executor's responsibility to finalize.
func (m *Manager) Stop() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.cron.Stop()
	}
	m.closed = true
	m.cancel()
	m.entries = make(map[int64]registration)
	return m.cron.Stop()
}

// LastError is for the Go application coordinator; never expose raw DB errors
// as diagnostic log text or user-facing Binding errors.
func (m *Manager) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastError
}

// Rebuild replaces all registrations with a fresh SQLite snapshot. It is the
// recovery path after a task's DB commit succeeded but Cron sync failed.
// On failure the previous jobs are removed (fail closed) and the caller can
// retry once the database/recovery problem has been corrected.
func (m *Manager) Rebuild(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuildLocked(ctx)
}

func (m *Manager) rebuildLocked(ctx context.Context) error {
	if m.closed {
		return ErrSchedulerClosed
	}
	m.clearLocked()
	rows, err := m.db.QueryContext(ctx, `
		SELECT t.id, t.next_due_at FROM tasks t
		JOIN rooms r ON r.id = t.room_id
		WHERE t.enabled = 1 AND t.deleted_at IS NULL AND r.deleted_at IS NULL
			AND t.next_due_at IS NOT NULL ORDER BY t.id`)
	if err != nil {
		return m.syncFailed(fmt.Errorf("load scheduled tasks: %w", err))
	}
	var plans []pendingPlan
	for rows.Next() {
		var plan pendingPlan
		if err := rows.Scan(&plan.id, &plan.due); err != nil {
			rows.Close()
			return m.syncFailed(fmt.Errorf("read scheduled task: %w", err))
		}
		plans = append(plans, plan)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return m.syncFailed(fmt.Errorf("read scheduled tasks: %w", err))
	}
	// Validate before adding jobs, so failure never leaves a partial schedule.
	for _, plan := range plans {
		if err := m.validatePlan(ctx, plan); err != nil {
			return m.syncFailed(err)
		}
	}
	for _, plan := range plans {
		if err := m.registerLocked(plan); err != nil {
			m.clearLocked()
			return m.syncFailed(err)
		}
	}
	m.lastError = nil
	return nil
}

// SyncTask reads the committed version of one task, replacing its previous
// timer even if a non-calendar edit retained the same due timestamp. Call
// after task mutations, never inside their SQLite transaction.
func (m *Manager) SyncTask(ctx context.Context, taskID int64) error {
	if taskID <= 0 {
		return task.ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrSchedulerClosed
	}
	m.removeLocked(taskID)
	var plan pendingPlan
	var due sql.NullInt64
	err := m.db.QueryRowContext(ctx, `
		SELECT t.id, t.next_due_at FROM tasks t
		JOIN rooms r ON r.id = t.room_id
		WHERE t.id = ? AND t.enabled = 1 AND t.deleted_at IS NULL
			AND r.deleted_at IS NULL`, taskID).Scan(&plan.id, &due)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return m.syncFailed(fmt.Errorf("read task schedule: %w", err))
	}
	if !due.Valid {
		return nil
	}
	plan.due = due.Int64
	if err := m.validatePlan(ctx, plan); err != nil {
		return m.syncFailed(err)
	}
	if err := m.registerLocked(plan); err != nil {
		return m.syncFailed(err)
	}
	return nil
}

func (m *Manager) validatePlan(ctx context.Context, plan pendingPlan) error {
	if !time.UnixMilli(plan.due).After(m.now()) {
		return fmt.Errorf("task %d: %w", plan.id, ErrRecoveryNeeded)
	}
	var kind schedule.Kind
	var timezone, clock string
	var localDate sql.NullString
	var weekdays sql.NullInt64
	if err := m.db.QueryRowContext(ctx, `
		SELECT schedule_kind, timezone, local_date, local_time, weekdays
		FROM tasks WHERE id = ?`, plan.id).Scan(&kind, &timezone, &localDate, &clock, &weekdays); err != nil {
		return fmt.Errorf("validate persisted schedule: %w", err)
	}
	instant, valid, err := schedule.NextAfter(schedule.Rule{
		Kind: kind, Timezone: timezone, LocalDate: localDate.String,
		LocalTime: clock, Weekdays: int(weekdays.Int64),
	}, time.UnixMilli(plan.due).Add(-time.Millisecond))
	if err != nil || !valid || instant.UnixMilli() != plan.due {
		return fmt.Errorf("task %d: invalid persisted scheduled occurrence", plan.id)
	}
	return nil
}

// oneShot tells Cron exactly when to wake, without allowing its calendar
// parser to diverge from A3 on nonexistent or repeated local wall times.
type oneShot time.Time

func (s oneShot) Next(after time.Time) time.Time {
	due := time.Time(s)
	if due.After(after) {
		return due
	}
	return time.Time{}
}

func (m *Manager) registerLocked(plan pendingPlan) error {
	// A due instant can pass between the DB read and Cron.Schedule. Do not
	// silently store a Cron entry whose Next() will already be zero.
	if !time.UnixMilli(plan.due).After(m.now()) {
		return fmt.Errorf("task %d: %w", plan.id, ErrRecoveryNeeded)
	}
	m.generation++
	gen := m.generation
	id := m.cron.Schedule(oneShot(time.UnixMilli(plan.due)), cron.FuncJob(func() {
		m.fire(plan.id, plan.due, gen)
	}))
	m.entries[plan.id] = registration{id: id, due: plan.due, generation: gen}
	return nil
}

func (m *Manager) clearLocked() {
	for _, reg := range m.entries {
		m.cron.Remove(reg.id)
	}
	m.entries = make(map[int64]registration)
}

func (m *Manager) removeLocked(taskID int64) {
	if reg, ok := m.entries[taskID]; ok {
		delete(m.entries, taskID)
		m.cron.Remove(reg.id)
	}
}

func (m *Manager) fire(taskID, due int64, gen uint64) {
	m.mu.Lock()
	reg, exists := m.entries[taskID]
	current := !m.closed && exists && reg.generation == gen && reg.due == due
	m.mu.Unlock()
	if !current {
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 6*time.Second)
	result, err := m.claim.Claim(ctx, taskID, due)
	cancel()
	if err != nil {
		m.report(fmt.Errorf("claim scheduled occurrence: %w", err))
		return
	}
	// Claim may have advanced the persistent cursor. Never run a second
	// calendar in this callback; replace the one-shot registration from DB.
	if err := m.SyncTask(m.ctx, taskID); err != nil && !errors.Is(err, ErrSchedulerClosed) {
		m.report(fmt.Errorf("sync after scheduled occurrence: %w", err))
	}
	if result.Claimed && result.Status == "waiting" && m.ctx.Err() == nil {
		if err := m.onReady(m.ctx, result); err != nil {
			m.report(fmt.Errorf("hand off scheduled execution: %w", err))
		}
	}
}

// syncFailed is called while holding mu; reporting to external callbacks is
// left to the caller after unlock to avoid callback reentrancy deadlocks.
func (m *Manager) syncFailed(err error) error {
	m.lastError = err
	if m.log != nil {
		m.log.Record(diagnostic.ScheduleSyncFailed)
	}
	return err
}

func (m *Manager) report(err error) {
	m.mu.Lock()
	m.lastError = err
	m.mu.Unlock()
	if m.onError != nil {
		m.onError(err)
	}
}

// Task mutations are performed by the existing task.Service, then Cron is
// synchronized strictly after commit. If synchronization fails, callers get
// the saved value (or completed deletion) and ErrScheduleSync, and can Rebuild.
func (m *Manager) CreateTask(ctx context.Context, input task.CreateInput) (task.Task, error) {
	saved, err := m.tasks.Create(ctx, input)
	if err != nil {
		return task.Task{}, err
	}
	return saved, m.syncAfterCommit(ctx, saved.ID)
}

func (m *Manager) UpdateTask(ctx context.Context, id int64, input task.CreateInput) (task.Task, error) {
	saved, err := m.tasks.Update(ctx, id, input)
	if err != nil {
		return task.Task{}, err
	}
	return saved, m.syncAfterCommit(ctx, id)
}

func (m *Manager) SetTaskEnabled(ctx context.Context, id int64, enabled bool) (task.Task, error) {
	saved, err := m.tasks.SetEnabled(ctx, id, enabled)
	if err != nil {
		return task.Task{}, err
	}
	return saved, m.syncAfterCommit(ctx, id)
}

func (m *Manager) DeleteTask(ctx context.Context, id int64) error {
	if err := m.tasks.Delete(ctx, id); err != nil {
		return err
	}
	return m.syncAfterCommit(ctx, id)
}

func (m *Manager) syncAfterCommit(ctx context.Context, id int64) error {
	if err := m.SyncTask(ctx, id); err != nil {
		return fmt.Errorf("%w: %w", ErrScheduleSync, err)
	}
	return nil
}
