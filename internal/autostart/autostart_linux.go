//go:build linux

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type linuxRegistration struct {
	configDir  func() (string, error)
	isAppImage bool
}

func newRegistration() registration {
	return &linuxRegistration{configDir: userConfigDir, isAppImage: os.Getenv("APPIMAGE") != ""}
}

func userConfigDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

func (r *linuxRegistration) path() (string, error) {
	dir, err := r.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", Identifier+".desktop"), nil
}

func (r *linuxRegistration) status(target string) (Status, error) {
	path, err := r.path()
	if err != nil {
		return Status{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	fields, valid := desktopFields(string(data))
	if fields["Hidden"] == "true" || fields["X-GNOME-Autostart-enabled"] == "false" {
		return Status{}, nil
	}
	exec, err := r.command(target)
	if err != nil {
		return Status{}, err
	}
	// We don't emit desktop/environment restrictions or extra arguments. An
	// externally edited or stale entry needs an explicit update by the user.
	valid = valid && fields["Type"] == "Application" && fields["Name"] != "" && fields["Exec"] == exec &&
		fields["OnlyShowIn"] == "" && fields["NotShowIn"] == "" && fields["TryExec"] == "" &&
		fields["DBusActivatable"] != "true" && fields["Terminal"] != "true"
	return Status{Enabled: valid, NeedsRepair: !valid}, nil
}

func (r *linuxRegistration) enable(target string) error {
	exec, err := r.command(target)
	if err != nil {
		return err
	}
	path, err := r.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body := "[Desktop Entry]\nType=Application\nName=dan-tick\nExec=" + exec +
		"\nTerminal=false\nHidden=false\nX-GNOME-Autostart-enabled=true\n"
	return writeAtomic(path, []byte(body))
}

func (r *linuxRegistration) command(target string) (string, error) {
	exec, err := desktopExec(target)
	if err != nil {
		return "", err
	}
	if r.isAppImage {
		// Extraction also works when the desktop session has no usable FUSE
		// mount. Always restart the persistent image, never its temporary mount.
		exec += " --appimage-extract-and-run"
	}
	return exec, nil
}

func (r *linuxRegistration) disable() error {
	path, err := r.path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func desktopFields(data string) (map[string]string, bool) {
	fields := make(map[string]string)
	active, found, valid := false, false, true
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			active = line == "[Desktop Entry]"
			if active {
				if found {
					valid = false
				}
				found = true
			}
			continue
		}
		if !active {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			valid = false
			continue
		}
		if _, duplicate := fields[key]; duplicate {
			valid = false
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields, valid && found
}

func desktopExec(target string) (string, error) {
	// GLib validates the executable token before expanding %% field codes, so
	// a literal percent in the path cannot reliably launch through GIO.
	if !filepath.IsAbs(target) || strings.ContainsAny(target, "=%") {
		return "", errors.New("unsupported desktop executable path")
	}
	var token strings.Builder
	for _, char := range target {
		if char < 0x20 || char == 0x7f {
			return "", errors.New("control character in desktop executable path")
		}
		switch char {
		case '"', '`', '$', '\\':
			token.WriteByte('\\')
			token.WriteRune(char)
		default:
			token.WriteRune(char)
		}
	}
	// Desktop Entry string unescaping precedes Exec token unquoting. Escape
	// every backslash again so literal backslashes survive both operations.
	return `"` + strings.ReplaceAll(token.String(), `\`, `\\`) + `"`, nil
}

func writeAtomic(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".dan-tick-autostart-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(0644); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace startup entry: %w", err)
	}
	return nil
}
