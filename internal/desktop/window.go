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

// GetStartupVisibility reports whether launching dan-tick starts with the main
// window hidden. The value read at application start governs the current run;
// changing it here only affects the next launch.
func (s *WindowService) GetStartupVisibility(ctx context.Context) (string, error) {
	value, err := s.settings.StartupVisibility(ctx)
	return value, protect(err)
}

func (s *WindowService) SetStartupVisibility(ctx context.Context, value string) error {
	if err := s.settings.SetStartupVisibility(ctx, value); err != nil {
		return protect(err)
	}
	s.changed(value)
	return nil
}

func (s *WindowService) Quit() {
	s.quit()
}
