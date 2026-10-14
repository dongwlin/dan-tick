package desktop

import (
	"context"

	"github.com/dongwlin/dan-tick/internal/autostart"
)

type autostartManager interface {
	Status(context.Context) (autostart.Status, error)
	SetEnabled(context.Context, bool) (autostart.Status, error)
}

// AutostartStatus reflects this user's startup registration. A stale or edited
// command is disabled in the UI until the user explicitly updates the entry.
type AutostartStatus struct {
	Enabled     bool `json:"enabled"`
	Supported   bool `json:"supported"`
	NeedsRepair bool `json:"needsRepair"`
}

type AutostartService struct {
	manager autostartManager
	changed func(string)
}

func NewAutostartService(manager *autostart.Manager, changed func(string)) *AutostartService {
	return &AutostartService{manager: manager, changed: changed}
}

func (s *AutostartService) GetAutostartStatus(ctx context.Context) (AutostartStatus, error) {
	status, err := s.manager.Status(ctx)
	return autostartStatus(status), protect(err)
}

func (s *AutostartService) SetAutostartEnabled(ctx context.Context, enabled bool) (AutostartStatus, error) {
	status, err := s.manager.SetEnabled(ctx, enabled)
	if err == nil && s.changed != nil {
		s.changed(EventApplicationChanged)
	}
	return autostartStatus(status), protect(err)
}

func autostartStatus(status autostart.Status) AutostartStatus {
	return AutostartStatus{Enabled: status.Enabled, Supported: status.Supported, NeedsRepair: status.NeedsRepair}
}
