// Package desktop exposes non-sensitive application configuration through Wails.
// It adapts frontend inputs and outputs; Go business services remain authoritative.
package desktop

import (
	"context"
	"errors"

	"github.com/dongwlin/dan-tick/internal/room"
	"github.com/dongwlin/dan-tick/internal/schedule"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/dongwlin/dan-tick/internal/task"
	"github.com/uptrace/bun"
)

// ConfigurationService is a Wails binding entry point, not a second business
// state store. All task mutations go through Manager to resync Cron after commit.
type ConfigurationService struct {
	rooms   *room.Service
	tasks   *task.Service
	manager *scheduler.Manager
	changed func()
}

// SetOnChange installs a notification hook. It carries no business payload.
func (s *ConfigurationService) SetOnChange(changed func()) { s.changed = changed }

func (s *ConfigurationService) notifyAfter(err error) {
	if (err == nil || errors.Is(err, scheduler.ErrScheduleSync)) && s.changed != nil {
		s.changed()
	}
}

func NewConfigurationService(db *bun.DB, client *room.Client, manager *scheduler.Manager) *ConfigurationService {
	return &ConfigurationService{
		rooms: room.NewService(db, client), tasks: task.NewService(db), manager: manager,
	}
}

type Room struct {
	ID             int64  `json:"id"`
	PlatformRoomID string `json:"platformRoomId"`
	Title          string `json:"title"`
	AnchorName     string `json:"anchorName"`
	CoverURL       string `json:"coverUrl"`
	Remark         string `json:"remark"`
	TaskCount      int    `json:"taskCount"`
}

func roomView(saved room.Room) Room {
	return Room{
		ID: saved.ID, PlatformRoomID: saved.PlatformRoomID,
		Title: saved.Title, AnchorName: saved.AnchorName, CoverURL: saved.CoverURL,
		Remark: saved.Remark, TaskCount: saved.TaskCount,
	}
}

type AddRoomInput struct {
	Value  string `json:"value"`  // Bilibili room ID or live.bilibili.com URL
	Remark string `json:"remark"` // Optional user remark; an empty remark keeps the stored one
}

type UpdateRoomInput struct {
	Remark string `json:"remark"`
}

type PageInput struct {
	Cursor *int64 `json:"cursor"`
	Limit  int    `json:"limit"`
}

func (p PageInput) size() int {
	if p.Limit == 0 {
		return 20
	}
	return p.Limit
}

type RoomPage struct {
	Items      []Room `json:"items"`
	NextCursor *int64 `json:"nextCursor"`
}

func (s *ConfigurationService) ListRoomPage(ctx context.Context, input PageInput) (RoomPage, error) {
	saved, next, err := s.rooms.ListPage(ctx, input.Cursor, input.size())
	if err != nil {
		return RoomPage{}, protect(err)
	}
	items := make([]Room, 0, len(saved))
	for _, item := range saved {
		items = append(items, roomView(item))
	}
	return RoomPage{Items: items, NextCursor: next}, nil
}

func (s *ConfigurationService) ListRooms(ctx context.Context) ([]Room, error) {
	saved, err := s.rooms.List(ctx)
	if err != nil {
		return nil, protect(err)
	}
	result := make([]Room, 0, len(saved))
	for _, item := range saved {
		result = append(result, roomView(item))
	}
	return result, nil
}

// RefreshRoomMetadata only updates descriptive fields for an active room.
// A true result requests a reload of authoritative list data.
func (s *ConfigurationService) RefreshRoomMetadata(ctx context.Context, id int64) (bool, error) {
	changed, err := s.rooms.RefreshMetadata(ctx, id)
	return changed, protect(err)
}

func (s *ConfigurationService) AddRoom(ctx context.Context, input AddRoomInput) (Room, error) {
	saved, err := s.rooms.Add(ctx, room.AddInput{Value: input.Value, Remark: input.Remark})
	if err != nil {
		return Room{}, protect(err)
	}
	s.notifyAfter(nil)
	return roomView(saved), nil
}

func (s *ConfigurationService) UpdateRoom(ctx context.Context, id int64, input UpdateRoomInput) (Room, error) {
	saved, err := s.rooms.UpdateRemark(ctx, id, input.Remark)
	if err != nil {
		return Room{}, protect(err)
	}
	s.notifyAfter(nil)
	return roomView(saved), nil
}

// Deleting a room disables all its tasks in the same SQLite transaction.
// Rebuild must happen AFTER commit so no deleted room retains a Cron entry.
func (s *ConfigurationService) DeleteRoom(ctx context.Context, id int64) error {
	if err := s.rooms.Delete(ctx, id); err != nil {
		return protect(err)
	}
	err := s.manager.Rebuild(ctx)
	if err != nil {
		err = scheduleSyncError(err)
	}
	s.notifyAfter(err)
	return protect(err)
}

type Plan struct {
	Kind      schedule.Kind `json:"kind"`
	Timezone  string        `json:"timezone"`
	LocalDate string        `json:"localDate"`
	LocalTime string        `json:"localTime"`
	Weekdays  int           `json:"weekdays"`
}

func (p Plan) rule() schedule.Rule {
	return schedule.Rule{
		Kind: p.Kind, Timezone: p.Timezone, LocalDate: p.LocalDate,
		LocalTime: p.LocalTime, Weekdays: p.Weekdays,
	}
}

type TaskInput struct {
	RoomID             int64     `json:"roomId"`
	Name               string    `json:"name"`
	Kind               task.Kind `json:"kind"`
	Messages           []string  `json:"messages"`
	IntervalSeconds    *int64    `json:"intervalSeconds"`
	MaxCount           *int64    `json:"maxCount"`
	MaxDurationSeconds *int64    `json:"maxDurationSeconds"`
	Schedule           Plan      `json:"schedule"`
	Enabled            bool      `json:"enabled"`
}

func (input TaskInput) taskInput() task.CreateInput {
	return task.CreateInput{
		RoomID: input.RoomID, Name: input.Name, Kind: input.Kind,
		Messages: input.Messages, IntervalSeconds: input.IntervalSeconds,
		MaxCount: input.MaxCount, MaxDurationSeconds: input.MaxDurationSeconds,
		Schedule: input.Schedule.rule(), Enabled: input.Enabled,
	}
}

// Task intentionally excludes Bun internals, deleted state and message row
// identifiers. The ordered message contents are needed for the edit form.
type Task struct {
	ID                 int64     `json:"id"`
	RoomID             int64     `json:"roomId"`
	Name               string    `json:"name"`
	Kind               task.Kind `json:"kind"`
	Messages           []string  `json:"messages"`
	IntervalSeconds    *int64    `json:"intervalSeconds"`
	MaxCount           *int64    `json:"maxCount"`
	MaxDurationSeconds *int64    `json:"maxDurationSeconds"`
	Schedule           Plan      `json:"schedule"`
	Enabled            bool      `json:"enabled"`
	NextDueAt          *int64    `json:"nextDueAt"`
	CreatedAt          int64     `json:"createdAt"`
	UpdatedAt          int64     `json:"updatedAt"`
}

func taskView(saved task.Task) Task {
	result := Task{
		ID: saved.ID, RoomID: saved.RoomID, Name: saved.Name,
		Kind: saved.Kind, Enabled: saved.Enabled,
		IntervalSeconds: saved.IntervalSeconds, MaxCount: saved.MaxCount,
		MaxDurationSeconds: saved.MaxDurationSeconds, NextDueAt: saved.NextDueAt,
		CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt,
		Schedule: Plan{
			Kind: saved.ScheduleKind, Timezone: saved.Timezone,
			LocalTime: saved.LocalTime,
		},
		Messages: make([]string, 0, len(saved.Messages)),
	}
	if saved.LocalDate != nil {
		result.Schedule.LocalDate = *saved.LocalDate
	}
	if saved.Weekdays != nil {
		result.Schedule.Weekdays = *saved.Weekdays
	}
	for _, message := range saved.Messages {
		result.Messages = append(result.Messages, message.Content)
	}
	return result
}

type TaskPageInput struct {
	Cursor *int64 `json:"cursor"`
	Limit  int    `json:"limit"`
	RoomID int64  `json:"roomId"`
	Search string `json:"search"`
}

type TaskPage struct {
	Items      []Task `json:"items"`
	NextCursor *int64 `json:"nextCursor"`
}

func (s *ConfigurationService) ListTaskPage(ctx context.Context, input TaskPageInput) (TaskPage, error) {
	saved, next, err := s.tasks.ListPage(ctx, input.RoomID, input.Search, input.Cursor, PageInput{Limit: input.Limit}.size())
	if err != nil {
		return TaskPage{}, protect(err)
	}
	items := make([]Task, 0, len(saved))
	for _, item := range saved {
		items = append(items, taskView(item))
	}
	return TaskPage{Items: items, NextCursor: next}, nil
}

// Legacy full-list bindings are retained for compatibility; new UI uses pages.
func (s *ConfigurationService) ListTasks(ctx context.Context, roomID int64) ([]Task, error) {
	saved, err := s.tasks.List(ctx, roomID)
	if err != nil {
		return nil, protect(err)
	}
	result := make([]Task, 0, len(saved))
	for _, item := range saved {
		result = append(result, taskView(item))
	}
	return result, nil
}

func (s *ConfigurationService) GetTask(ctx context.Context, id int64) (Task, error) {
	saved, err := s.tasks.Get(ctx, id)
	if err != nil {
		return Task{}, protect(err)
	}
	return taskView(saved), nil
}

func (s *ConfigurationService) CreateTask(ctx context.Context, input TaskInput) (Task, error) {
	saved, err := s.manager.CreateTask(ctx, input.taskInput())
	s.notifyAfter(err)
	return taskView(saved), protect(err)
}

func (s *ConfigurationService) UpdateTask(ctx context.Context, id int64, input TaskInput) (Task, error) {
	saved, err := s.manager.UpdateTask(ctx, id, input.taskInput())
	s.notifyAfter(err)
	return taskView(saved), protect(err)
}

func (s *ConfigurationService) SetTaskEnabled(ctx context.Context, id int64, enabled bool) (Task, error) {
	saved, err := s.manager.SetTaskEnabled(ctx, id, enabled)
	s.notifyAfter(err)
	return taskView(saved), protect(err)
}

func (s *ConfigurationService) DeleteTask(ctx context.Context, id int64) error {
	err := s.manager.DeleteTask(ctx, id)
	s.notifyAfter(err)
	return protect(err)
}
