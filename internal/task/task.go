// Package task persists task definitions and their ordered messages.
// Plan calculations are shared with the scheduler; no HTTP or Wails dependency
// is needed to create or read local task configuration.
package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/uptrace/bun"
)

var (
	ErrInvalidTask     = errors.New("invalid task configuration")
	ErrRoomUnavailable = errors.New("target room is not available")
	ErrNotFound        = errors.New("task not found")
)

type Kind string

const (
	Single Kind = "single"
	Loop   Kind = "loop"
)

// CreateInput contains only user-editable fields. Zero-value Timezone uses
// Asia/Shanghai; Weekdays uses Monday bit 0 through Sunday bit 6.
type CreateInput struct {
	RoomID             int64
	Name               string
	Kind               Kind
	Messages           []string
	IntervalSeconds    *int64
	MaxCount           *int64
	MaxDurationSeconds *int64
	Schedule           schedule.Rule
	Enabled            bool
}

type Message struct {
	bun.BaseModel `bun:"table:task_messages,alias:m"`
	ID            int64  `bun:"id,pk"`
	TaskID        int64  `bun:"task_id"`
	Position      int    `bun:"position"`
	Content       string `bun:"content"`
}

// Task reflects the persisted SQLite fields. Times are UTC Unix milliseconds,
// and nullable fields remain nil rather than using ambiguous zero values.
type Task struct {
	bun.BaseModel      `bun:"table:tasks,alias:t"`
	ID                 int64         `bun:"id,pk"`
	RoomID             int64         `bun:"room_id"`
	Name               string        `bun:"name"`
	Kind               Kind          `bun:"kind"`
	Enabled            bool          `bun:"enabled"`
	IntervalSeconds    *int64        `bun:"interval_seconds"`
	MaxCount           *int64        `bun:"max_count"`
	MaxDurationSeconds *int64        `bun:"max_duration_seconds"`
	ScheduleKind       schedule.Kind `bun:"schedule_kind"`
	Timezone           string        `bun:"timezone"`
	LocalDate          *string       `bun:"local_date"`
	LocalTime          string        `bun:"local_time"`
	Weekdays           *int          `bun:"weekdays"`
	NextDueAt          *int64        `bun:"next_due_at"`
	CreatedAt          int64         `bun:"created_at"`
	UpdatedAt          int64         `bun:"updated_at"`
	DeletedAt          *int64        `bun:"deleted_at"`
	Messages           []Message     `bun:"-"`
}

type Service struct {
	db  *bun.DB
	now func() time.Time
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// Create validates the complete configuration, then saves the task and all
// messages atomically. No external lookup or scheduler operation runs in the
// transaction. B4 will add editing and task status changes.
func (s *Service) Create(ctx context.Context, input CreateInput) (Task, error) {
	now := s.now().UTC()
	input, due, err := prepare(input, now)
	if err != nil {
		return Task{}, err
	}
	var created Task
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var active bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM rooms WHERE id = ? AND deleted_at IS NULL)", input.RoomID,
		).Scan(&active); err != nil {
			return fmt.Errorf("check task room: %w", err)
		}
		if !active {
			return ErrRoomUnavailable
		}
		var localDate any
		if input.Schedule.Kind == schedule.Once {
			localDate = input.Schedule.LocalDate
		}
		var weekdays any
		if input.Schedule.Kind == schedule.Weekly {
			weekdays = input.Schedule.Weekdays
		}
		var id int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO tasks (room_id, name, kind, enabled, interval_seconds,
				max_count, max_duration_seconds, schedule_kind, timezone, local_date,
				local_time, weekdays, next_due_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			RETURNING id`, input.RoomID, input.Name, input.Kind, input.Enabled,
			input.IntervalSeconds, input.MaxCount, input.MaxDurationSeconds,
			input.Schedule.Kind, input.Schedule.Timezone, localDate,
			input.Schedule.LocalTime, weekdays, due, now.UnixMilli(), now.UnixMilli(),
		).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert task: %w", err)
		}
		for position, content := range input.Messages {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO task_messages (task_id, position, content)
				VALUES (?, ?, ?)`, id, position, content); err != nil {
				return fmt.Errorf("insert task message %d: %w", position, err)
			}
		}
		created, err = getTask(ctx, tx, id)
		return err
	})
	if err != nil {
		return Task{}, err
	}
	return created, nil
}

// Get loads an active task and its messages in their stored order.
func (s *Service) Get(ctx context.Context, id int64) (Task, error) {
	if id <= 0 {
		return Task{}, ErrNotFound
	}
	var task Task
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		task, err = getTask(ctx, tx, id)
		return err
	})
	if err != nil {
		return Task{}, err
	}
	return task, nil
}

func getTask(ctx context.Context, db bun.IDB, id int64) (Task, error) {
	var task Task
	err := activeTasks(db.NewSelect().Model(&task)).Where("t.id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("get task: %w", err)
	}
	tasks := []Task{task}
	if err := loadMessages(ctx, db, tasks); err != nil {
		return Task{}, err
	}
	return tasks[0], nil
}

// List returns nondeleted tasks for all rooms or an optional specific room.
// Pass roomID=0 for all rooms. Deleted rooms and tasks are never listed.
func (s *Service) List(ctx context.Context, roomID int64) ([]Task, error) {
	if roomID < 0 {
		return nil, ErrRoomUnavailable
	}
	tasks := make([]Task, 0)
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		query := activeTasks(tx.NewSelect().Model(&tasks)).OrderExpr("t.id DESC")
		if roomID > 0 {
			query = query.Where("t.room_id = ?", roomID)
		}
		if err := query.Scan(ctx); err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}
		return loadMessages(ctx, tx, tasks)
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func activeTasks(query *bun.SelectQuery) *bun.SelectQuery {
	return query.Where("t.deleted_at IS NULL").Where(`EXISTS (
		SELECT 1 FROM rooms r WHERE r.id = t.room_id AND r.deleted_at IS NULL
	)`)
}

func loadMessages(ctx context.Context, db bun.IDB, tasks []Task) error {
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]int64, len(tasks))
	byID := make(map[int64]int, len(tasks))
	for i := range tasks {
		ids[i] = tasks[i].ID
		byID[tasks[i].ID] = i
		tasks[i].Messages = make([]Message, 0)
	}
	var messages []Message
	if err := db.NewSelect().Model(&messages).
		Where("m.task_id IN (?)", bun.In(ids)).
		OrderExpr("m.task_id, m.position, m.id").Scan(ctx); err != nil {
		return fmt.Errorf("read task messages: %w", err)
	}
	for _, message := range messages {
		index := byID[message.TaskID]
		tasks[index].Messages = append(tasks[index].Messages, message)
	}
	return nil
}

func prepare(input CreateInput, now time.Time) (CreateInput, *int64, error) {
	if input.RoomID <= 0 {
		return input, nil, fmt.Errorf("%w: room_id", ErrInvalidTask)
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || !printable(input.Name) {
		return input, nil, fmt.Errorf("%w: name", ErrInvalidTask)
	}
	if input.Schedule.Timezone == "" {
		input.Schedule.Timezone = "Asia/Shanghai"
	}
	input.Messages = cleanMessages(input.Messages)
	switch input.Kind {
	case Single:
		if len(input.Messages) != 1 || input.IntervalSeconds != nil || input.MaxCount != nil || input.MaxDurationSeconds != nil {
			return input, nil, fmt.Errorf("%w: single requires exactly one message and no loop settings", ErrInvalidTask)
		}
	case Loop:
		if len(input.Messages) == 0 || input.IntervalSeconds == nil || *input.IntervalSeconds <= 0 ||
			(input.MaxCount == nil && input.MaxDurationSeconds == nil) ||
			(input.MaxCount != nil && *input.MaxCount <= 0) ||
			(input.MaxDurationSeconds != nil && *input.MaxDurationSeconds <= 0) {
			return input, nil, fmt.Errorf("%w: loop requires messages, positive interval and a positive stop condition", ErrInvalidTask)
		}
	default:
		return input, nil, fmt.Errorf("%w: kind", ErrInvalidTask)
	}
	for _, message := range input.Messages {
		if !printable(message) {
			return input, nil, fmt.Errorf("%w: message contains invalid text", ErrInvalidTask)
		}
	}
	next, exists, err := schedule.NextAfter(input.Schedule, now)
	if err != nil {
		return input, nil, fmt.Errorf("%w: %w", ErrInvalidTask, err)
	}
	if !input.Enabled {
		return input, nil, nil
	}
	if !exists {
		return input, nil, fmt.Errorf("%w: enabled plan has no future occurrence", ErrInvalidTask)
	}
	due := next.UTC().UnixMilli()
	return input, &due, nil
}

func cleanMessages(messages []string) []string {
	cleaned := make([]string, 0, len(messages))
	for _, text := range messages {
		text = strings.TrimSpace(text)
		if text != "" {
			cleaned = append(cleaned, text)
		}
	}
	return cleaned
}

func printable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
