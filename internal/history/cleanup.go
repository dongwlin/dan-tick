package history

import (
	"context"
	"fmt"
	"time"
)

// Retention is the rolling UTC duration for completed execution history. It is
// independent of the diagnostic log retention policy.
const Retention = 30 * 24 * time.Hour

// CleanupExpired removes only terminal executions finished strictly before the
// retention cutoff. SQLite cascades each deletion to its send_attempts in the
// same atomic statement; active executions and task definitions are untouched.
// A terminal execution without a finished_at is kept for investigation.
func (s *Service) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	cutoff := now.Add(-Retention).UnixMilli()
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM executions
		WHERE status IN ('completed', 'stopped', 'failed', 'missed', 'interrupted')
			AND finished_at IS NOT NULL AND finished_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("clean expired execution history: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count cleaned execution history: %w", err)
	}
	return deleted, nil
}
