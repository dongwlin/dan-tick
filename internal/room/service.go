package room

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

var (
	ErrAlreadyAdded = errors.New("live room already added")
	ErrNotSaved     = errors.New("live room not saved")
	ErrRoomBusy     = errors.New("live room has active executions")
	ErrInvalidPage  = errors.New("invalid room page")
)

// Room represents a saved room. TaskCount includes only nondeleted tasks.
type Room struct {
	bun.BaseModel     `bun:"table:rooms,alias:r"`
	ID                int64  `bun:"id,pk,autoincrement"`
	PlatformRoomID    string `bun:"platform_room_id"`
	Title             string `bun:"title"`
	AnchorName        string `bun:"anchor_name"`
	Remark            string `bun:"remark"`
	CoverURL          string `bun:"cover_url"`
	MetadataCheckedAt int64  `bun:"metadata_checked_at"`
	CreatedAt         int64  `bun:"created_at"`
	UpdatedAt         int64  `bun:"updated_at"`
	DeletedAt         *int64 `bun:"deleted_at"`
	TaskCount         int    `bun:"task_count,scanonly"`
}

// Service manages the local room list. External lookup always finishes before
// entering a SQLite transaction; no Wails binding or scheduler is required.
type Service struct {
	db     *bun.DB
	client *Client
	now    func() time.Time
}

func NewService(db *bun.DB, client *Client) *Service {
	return &Service{db: db, client: client, now: time.Now}
}

// List returns active rooms, newest first, without performing network requests.
func (s *Service) List(ctx context.Context) ([]Room, error) {
	rooms := make([]Room, 0)
	err := roomQuery(s.db.NewSelect().Model(&rooms)).
		Where("r.deleted_at IS NULL").OrderExpr("r.id DESC").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	return rooms, nil
}

// ListPage uses the immutable row ID as a descending cursor. New rows inserted
// while paging cannot shift the boundary and make earlier rows repeat.
func (s *Service) ListPage(ctx context.Context, cursor *int64, limit int) ([]Room, *int64, error) {
	if (cursor != nil && *cursor <= 0) || limit < 1 || limit > 100 {
		return nil, nil, ErrInvalidPage
	}
	rooms := make([]Room, 0)
	query := roomQuery(s.db.NewSelect().Model(&rooms)).
		Where("r.deleted_at IS NULL").OrderExpr("r.id DESC").Limit(limit + 1)
	if cursor != nil {
		query = query.Where("r.id < ?", *cursor)
	}
	if err := query.Scan(ctx); err != nil {
		return nil, nil, fmt.Errorf("list room page: %w", err)
	}
	if len(rooms) <= limit {
		return rooms, nil, nil
	}
	rooms = rooms[:limit]
	next := rooms[len(rooms)-1].ID
	return rooms, &next, nil
}

const metadataRefreshInterval = 30 * time.Minute

// RefreshMetadata updates display-only fields for a saved room when its last
// attempt is stale. Requests run outside database transactions; a guarded
// update prevents a slower concurrent response from replacing newer data.
// Missing upstream fields never erase an existing title or anchor name, and
// an outage is recorded as an attempt so reopening the page does not retry
// immediately. User remarks are never touched.
func (s *Service) RefreshMetadata(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, ErrNotSaved
	}
	var saved Room
	err := s.db.NewSelect().Model(&saved).Where("r.id = ? AND r.deleted_at IS NULL", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotSaved
	}
	if err != nil {
		return false, fmt.Errorf("read room metadata: %w", err)
	}
	now := s.now().UTC().UnixMilli()
	cutoff := now - metadataRefreshInterval.Milliseconds()
	if saved.MetadataCheckedAt > cutoff {
		return false, nil
	}
	info, err := s.client.Resolve(ctx, saved.PlatformRoomID)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	// Never alter the room identity from a refresh result.
	valid := err == nil && info.RoomID == saved.PlatformRoomID
	title, anchor, cover := saved.Title, saved.AnchorName, saved.CoverURL
	if valid {
		if info.Title != "" {
			title = info.Title
		}
		if info.AnchorName != "" {
			anchor = info.AnchorName
		}
		cover = info.CoverURL
	}
	changed := title != saved.Title || anchor != saved.AnchorName || cover != saved.CoverURL
	result, err := s.db.ExecContext(ctx, `
		UPDATE rooms SET title = ?, anchor_name = ?, cover_url = ?, metadata_checked_at = ?,
			updated_at = CASE WHEN title != ? OR anchor_name != ? OR cover_url != ? THEN ? ELSE updated_at END
		WHERE id = ? AND deleted_at IS NULL AND metadata_checked_at <= ?`,
		title, anchor, cover, now, title, anchor, cover, now, id, cutoff)
	if err != nil {
		return false, fmt.Errorf("save room metadata: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check room metadata update: %w", err)
	}
	return affected > 0 && changed, nil
}

// Add validates the input with Bilibili and persists its canonical room ID.
// Already-active IDs are rejected without overwriting user data. Readding a
// deleted room restores its original row and remark, but NOT its deleted tasks.
func (s *Service) Add(ctx context.Context, input string) (Room, error) {
	info, err := s.client.Resolve(ctx, input)
	if err != nil {
		return Room{}, err
	}
	now := s.now().UTC().UnixMilli()
	var saved Room
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var id int64
		// SQLite's unique constraint serializes simultaneous additions; only a
		// previously deleted row can be changed by the conflict handler.
		err := tx.QueryRowContext(ctx, `
			INSERT INTO rooms (platform_room_id, title, anchor_name, cover_url, metadata_checked_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (platform_room_id) DO UPDATE SET
				title = CASE WHEN excluded.title != '' THEN excluded.title ELSE rooms.title END,
				anchor_name = CASE WHEN excluded.anchor_name != '' THEN excluded.anchor_name ELSE rooms.anchor_name END,
				cover_url = excluded.cover_url,
				metadata_checked_at = excluded.metadata_checked_at,
				updated_at = excluded.updated_at,
				deleted_at = NULL
			WHERE rooms.deleted_at IS NOT NULL
			RETURNING id`, info.RoomID, info.Title, info.AnchorName, info.CoverURL, now, now, now).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAlreadyAdded
		}
		if err != nil {
			return fmt.Errorf("save room: %w", err)
		}
		saved, err = getRoom(ctx, tx, id)
		return err
	})
	if err != nil {
		return Room{}, err
	}
	return saved, nil
}

// UpdateRemark changes only the user-controlled remark of an active room.
func (s *Service) UpdateRemark(ctx context.Context, id int64, remark string) (Room, error) {
	if id <= 0 {
		return Room{}, ErrNotSaved
	}
	var saved Room
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE rooms SET remark = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, strings.TrimSpace(remark), s.now().UTC().UnixMilli(), id)
		if err != nil {
			return fmt.Errorf("update room remark: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check updated room: %w", err)
		}
		if count == 0 {
			return ErrNotSaved
		}
		saved, err = getRoom(ctx, tx, id)
		return err
	})
	if err != nil {
		return Room{}, err
	}
	return saved, nil
}

// Delete logically deletes a room and its tasks in a single transaction. A
// running task must first be stopped by the executor (added in a later unit);
// B2 never marks a live execution as stopped without actually cancelling it.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrNotSaved
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := s.now().UTC().UnixMilli()
		result, err := tx.ExecContext(ctx, `
			UPDATE rooms SET deleted_at = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`, now, now, id)
		if err != nil {
			return fmt.Errorf("delete room: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check deleted room: %w", err)
		}
		if count == 0 {
			return ErrNotSaved
		}
		var busy bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM executions e
				JOIN tasks t ON t.id = e.task_id
				WHERE t.room_id = ? AND e.status IN ('waiting', 'queued', 'running')
			)`, id).Scan(&busy); err != nil {
			return fmt.Errorf("check active room executions: %w", err)
		}
		if busy {
			return ErrRoomBusy
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE tasks SET enabled = 0, next_due_at = NULL, deleted_at = ?, updated_at = ?
			WHERE room_id = ? AND deleted_at IS NULL`, now, now, id); err != nil {
			return fmt.Errorf("delete room tasks: %w", err)
		}
		return nil
	})
}

func roomQuery(q *bun.SelectQuery) *bun.SelectQuery {
	return q.ColumnExpr("r.*").ColumnExpr(`(
		SELECT COUNT(*) FROM tasks t WHERE t.room_id = r.id AND t.deleted_at IS NULL
	) AS task_count`)
}

func getRoom(ctx context.Context, db bun.IDB, id int64) (Room, error) {
	var room Room
	if err := roomQuery(db.NewSelect().Model(&room)).Where("r.id = ?", id).Scan(ctx); err != nil {
		return Room{}, fmt.Errorf("read room: %w", err)
	}
	return room, nil
}
