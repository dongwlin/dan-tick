package desktop

import (
	"context"
	"math"

	"github.com/dongwlin/dan-tick/internal/history"
)

// HistoryService is the read-only Wails boundary for persisted execution
// snapshots. It never queries mutable task definitions or returns raw errors.
type HistoryService struct{ history *history.Service }

func NewHistoryService(service *history.Service) *HistoryService {
	return &HistoryService{history: service}
}

type HistoryPage struct {
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	Total      int64               `json:"total"`
	TotalPages int64               `json:"totalPages"`
	Items      []history.Execution `json:"items"`
}

// ListExecutionHistory accepts 1-based page numbers. Page size is limited by
// the history service, while the UI derives its preferred size from height.
func (s *HistoryService) ListExecutionHistory(ctx context.Context, page, pageSize int) (HistoryPage, error) {
	if page < 1 || pageSize < 1 || pageSize > history.MaxPageSize || page-1 > math.MaxInt/pageSize {
		return HistoryPage{}, protect(history.ErrInvalidPage)
	}
	result, err := s.history.List(ctx, (page-1)*pageSize, pageSize)
	if err != nil {
		return HistoryPage{}, protect(err)
	}
	return HistoryPage{
		Page: page, PageSize: pageSize, Total: result.Total,
		TotalPages: (result.Total + int64(pageSize) - 1) / int64(pageSize), Items: result.Items,
	}, nil
}

func (s *HistoryService) GetExecutionHistory(ctx context.Context, executionID int64) (history.Detail, error) {
	detail, err := s.history.Get(ctx, executionID)
	if err != nil {
		return history.Detail{}, protect(err)
	}
	return detail, nil
}
