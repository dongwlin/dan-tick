package desktop

import (
	"context"

	"github.com/dongwlin/dan-tick/internal/settings"
)

// WindowService exposes only non-sensitive desktop preferences and an explicit
// quit request. Native close events use the same callbacks as the frontend.
type WindowService struct {
	settings *settings.Service
	changed  func(string)
	quit     func()
}

func NewWindowService(settings *settings.Service, changed func(string), quit func()) *WindowService {
	return &WindowService{settings: settings, changed: changed, quit: quit}
}

func (s *WindowService) GetCloseBehavior(ctx context.Context) (string, error) {
	value, err := s.settings.CloseBehavior(ctx)
	return value, protect(err)
}

func (s *WindowService) SetCloseBehavior(ctx context.Context, behavior string) error {
	if err := s.settings.SetCloseBehavior(ctx, behavior); err != nil {
		return protect(err)
	}
	s.changed(behavior)
	return nil
}

func (s *WindowService) Quit() {
	s.quit()
}
