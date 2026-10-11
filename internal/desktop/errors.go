package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dongwlin/dan-tick/internal/room"
	"github.com/dongwlin/dan-tick/internal/scheduler"
	"github.com/dongwlin/dan-tick/internal/task"
)

type safeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type publicError struct{ details safeError }

func (e publicError) Error() string { return e.details.Message }

// Wails puts error.Error() in the top-level rejected call message, even when
// MarshalError customises its cause. Sanitize BOTH paths at the service edge.
func protect(err error) error {
	if err == nil {
		return nil
	}
	return publicError{details: classify(err)}
}

func classify(err error) safeError {
	reply := safeError{Code: "internal", Message: "操作失败，请重试或查看诊断日志"}
	switch {
	case errors.Is(err, scheduler.ErrScheduleSync), errors.Is(err, scheduler.ErrRecoveryNeeded):
		reply = safeError{"schedule_sync", "修改已保存，但定时调度同步失败，请重新启动应用后检查"}
	case errors.Is(err, room.ErrInvalidInput), errors.Is(err, task.ErrInvalidTask):
		reply = safeError{"invalid_input", "输入无效，请检查填写内容"}
	case errors.Is(err, room.ErrAlreadyAdded):
		reply = safeError{"already_exists", "该直播间已添加"}
	case errors.Is(err, room.ErrNotSaved), errors.Is(err, task.ErrNotFound), errors.Is(err, room.ErrNotFound):
		reply = safeError{"not_found", "指定直播间或任务不存在"}
	case errors.Is(err, task.ErrRoomUnavailable):
		reply = safeError{"room_unavailable", "目标直播间不存在或已删除"}
	case errors.Is(err, room.ErrRoomBusy), errors.Is(err, task.ErrTaskBusy):
		reply = safeError{"conflict", "存在正在执行的任务，请先停止执行"}
	case errors.Is(err, scheduler.ErrSchedulerClosed):
		reply = safeError{"unavailable", "应用正在关闭，暂时无法更改任务"}
	case errors.Is(err, room.ErrUnavailable), errors.Is(err, room.ErrInvalidResponse):
		reply = safeError{"upstream_unavailable", "直播间验证暂不可用，请稍后再试"}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		reply = safeError{"cancelled", "操作已取消或超时"}
	}
	return reply
}

// MarshalError is used by Wails ServiceOptions.MarshalError. The frontend can
// read the stable code from the rejected call's cause without parsing strings.
func MarshalError(err error) []byte {
	var exposed publicError
	reply := safeError{Code: "internal", Message: "操作失败，请重试或查看诊断日志"}
	if errors.As(err, &exposed) {
		reply = exposed.details
	}
	encoded, _ := json.Marshal(reply)
	return encoded
}

func scheduleSyncError(err error) error {
	return fmt.Errorf("%w: %w", scheduler.ErrScheduleSync, err)
}
