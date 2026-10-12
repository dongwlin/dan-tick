package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/dongwlin/dan-tick/internal/diagnostic"
	"github.com/dongwlin/dan-tick/internal/storage"
)

// DiagnosticsService delegates native UI actions to the Wails application.
// Paths and arbitrary underlying errors never cross the binding boundary.
type DiagnosticsService struct {
	logger     *diagnostic.Logger
	openFolder func(string) error
	savePath   func() (string, error)
	version    string
}

func NewDiagnosticsService(logger *diagnostic.Logger, openFolder func(string) error, savePath func() (string, error), version string) *DiagnosticsService {
	return &DiagnosticsService{logger: logger, openFolder: openFolder, savePath: savePath, version: version}
}

type DiagnosticsStatus struct {
	Directory string `json:"directory"`
	Degraded  bool   `json:"degraded"`
	Warning   string `json:"warning"`
}

func (s *DiagnosticsService) GetDiagnosticsStatus() DiagnosticsStatus {
	status := s.logger.Status()
	return DiagnosticsStatus{Directory: status.Directory, Degraded: status.Degraded, Warning: status.Warning}
}

func (s *DiagnosticsService) OpenLogDirectory() error {
	path := s.logger.Status().Directory
	if err := os.MkdirAll(path, 0700); err != nil {
		return protect(err)
	}
	return protect(s.openFolder(path))
}

func (s *DiagnosticsService) OpenDataDirectory() error {
	return protect(s.openFolder(filepath.Dir(storage.DefaultPath())))
}

type DiagnosticExportResult struct {
	Cancelled bool   `json:"cancelled"`
	Filename  string `json:"filename"`
}

func (s *DiagnosticsService) ExportDiagnostics() (DiagnosticExportResult, error) {
	path, err := s.savePath()
	if err != nil {
		return DiagnosticExportResult{}, protect(err)
	}
	if path == "" {
		return DiagnosticExportResult{Cancelled: true}, nil
	}
	if err := s.logger.Export(path, s.version); err != nil {
		return DiagnosticExportResult{}, protect(err)
	}
	return DiagnosticExportResult{Filename: filepath.Base(path)}, nil
}

func isUnsafeExport(err error) bool { return errors.Is(err, diagnostic.ErrUnsafeExport) }
