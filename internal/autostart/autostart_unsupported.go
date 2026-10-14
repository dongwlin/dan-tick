//go:build !linux && !windows

package autostart

func newRegistration() registration { return nil }
