package desktop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dongwlin/dan-tick/internal/history"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/dongwlin/dan-tick/internal/settings"
	"github.com/uptrace/bun"
)

const EventApplicationChanged = "dan-tick:application-changed"

// ApplicationService presents settings and a bounded dashboard snapshot from
// SQLite. No scheduler state is held in the frontend or duplicated here.
type ApplicationService struct {
	db       *bun.DB
	settings *settings.Service
	manager  *scheduler.Manager
	changed  func(string)
}

func NewApplicationService(db *bun.DB, manager *scheduler.Manager, changed func(string)) *ApplicationService {
	return &ApplicationService{db: db, settings: settings.NewService(db), manager: manager, changed: changed}
}

type Preferences struct {
	DefaultTimezone   string `json:"defaultTimezone"`
	SchedulingEnabled bool   `json:"schedulingEnabled"`
}

func (s *ApplicationService) GetPreferences(ctx context.Context) (Preferences, error) {
	values, err := s.settings.Get(ctx)
	if err != nil {
		return Preferences{}, protect(err)
	}
	return Preferences{DefaultTimezone: values.DefaultTimezone, SchedulingEnabled: values.SchedulingEnabled}, nil
}

func (s *ApplicationService) SetDefaultTimezone(ctx context.Context, timezone string) error {
	if err := s.settings.SetDefaultTimezone(ctx, timezone); err != nil {
		return protect(err)
	}
	s.notify()
	return nil
}

func (s *ApplicationService) SetSchedulingEnabled(ctx context.Context, enabled bool) error {
	err := s.manager.SetSchedulingEnabled(ctx, enabled)
	// The setting can be committed even when Cron sync fails. Notify consumers
	// to refetch instead of presenting the optimistic value as authoritative.
	if err == nil || isScheduleSync(err) {
		s.notify()
	}
	return protect(err)
}

func (s *ApplicationService) notify() {
	if s.changed != nil {
		s.changed(EventApplicationChanged)
	}
}

type UpcomingTask struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Room      string `json:"room"`
	NextDueAt int64  `json:"nextDueAt"`
	Timezone  string `json:"timezone"`
}

type RecentFailure struct {
	ID       int64  `json:"id"`
	TaskName string `json:"taskName"`
	Status   string `json:"status"`
	Time     int64  `json:"time"`
}

type Overview struct {
	TotalTasks       int64               `json:"totalTasks"`
	EnabledTasks     int64               `json:"enabledTasks"`
	ActiveExecutions int64               `json:"activeExecutions"`
	RoomCount        int64               `json:"roomCount"`
	Upcoming         []UpcomingTask      `json:"upcoming"`
	Recent           []history.Execution `json:"recent"`
	LatestFailure    *RecentFailure      `json:"latestFailure"`
}

func (s *ApplicationService) GetOverview(ctx context.Context) (Overview, error) {
	result := Overview{Upcoming: []UpcomingTask{}, Recent: []history.Execution{}}
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(t.enabled), 0) FROM tasks t
			JOIN rooms r ON r.id = t.room_id WHERE t.deleted_at IS NULL AND r.deleted_at IS NULL`).
			Scan(&result.TotalTasks, &result.EnabledTasks); err != nil {
			return fmt.Errorf("count tasks: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM rooms WHERE deleted_at IS NULL`).Scan(&result.RoomCount); err != nil {
			return fmt.Errorf("count rooms: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE status IN ('waiting','queued','running')`).
			Scan(&result.ActiveExecutions); err != nil {
			return fmt.Errorf("count active executions: %w", err)
		}
		rows, err := tx.QueryContext(ctx, `SELECT t.id, t.name, t.kind, r.platform_room_id, t.next_due_at, t.timezone
			FROM tasks t JOIN rooms r ON r.id = t.room_id
			WHERE t.deleted_at IS NULL AND r.deleted_at IS NULL AND t.enabled = 1 AND t.next_due_at IS NOT NULL
			ORDER BY t.next_due_at ASC, t.id ASC LIMIT 5`)
		if err != nil {
			return fmt.Errorf("load upcoming tasks: %w", err)
		}
		for rows.Next() {
			var item UpcomingTask
			if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Room, &item.NextDueAt, &item.Timezone); err != nil {
				rows.Close()
				return err
			}
			result.Upcoming = append(result.Upcoming, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		var failure RecentFailure
		err = tx.QueryRowContext(ctx, `SELECT id, task_name_snapshot, status, created_at FROM executions
			WHERE status IN ('failed','missed','interrupted') ORDER BY created_at DESC, id DESC LIMIT 1`).
			Scan(&failure.ID, &failure.TaskName, &failure.Status, &failure.Time)
		if err == nil {
			result.LatestFailure = &failure
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("load latest failure: %w", err)
		}
		return nil
	})
	if err != nil {
		return Overview{}, protect(err)
	}
	recent, err := history.NewService(s.db).List(ctx, 0, 5)
	if err != nil {
		return Overview{}, protect(err)
	}
	result.Recent = recent.Items
	return result, nil
}

func isScheduleSync(err error) bool { return errors.Is(err, scheduler.ErrScheduleSync) }
