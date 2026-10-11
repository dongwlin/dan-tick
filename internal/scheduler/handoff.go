package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
)

// A failed dispatch must not strand an execution that nobody owns. Finalize
// only an untouched waiting row: queued/running work and any persisted attempt
// remain the executor's responsibility, since sending may already have begun.
func (m *Manager) finishUnacceptedHandoff(taskID, executionID int64, cancelled bool) error {
	status, code, event := "failed", "handoff_failed", diagnostic.ExecutionFailed
	if cancelled {
		status, code, event = "stopped", "cancelled", diagnostic.ExecutionStopped
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(m.ctx), 6*time.Second)
	defer cancel()
	result, err := m.db.ExecContext(ctx, `
		UPDATE executions SET status = ?, finished_at = ?, error_code = ?, error_message = NULL
		WHERE id = ? AND task_id = ? AND trigger_kind = 'scheduled' AND status = 'waiting'
			AND NOT EXISTS (SELECT 1 FROM send_attempts WHERE execution_id = executions.id)`,
		status, m.now().UTC().UnixMilli(), code, executionID, taskID)
	if err != nil {
		if m.log != nil {
			m.log.RecordExecution(diagnostic.ExecutionPersistFailed, executionID, taskID, "")
		}
		return fmt.Errorf("finalize unaccepted scheduled execution: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check scheduled handoff finalization: %w", err)
	}
	if affected > 0 && m.log != nil {
		m.log.RecordExecution(event, executionID, taskID, code)
	}
	return nil
}
