package main

import (
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ShortcutFailure is one rejected or failed iOS Shortcut / upload API call.
type ShortcutFailure struct {
	Time     string `json:"time"`
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail"`
	ClientIP string `json:"clientIP"`
}

const maxShortcutFailures = 50

func (a *App) logShortcutFailure(path, reason, detail, clientIP string) {
	a.failuresMu.Lock()
	defer a.failuresMu.Unlock()
	if a.shortcutFailures == nil {
		a.shortcutFailures = make([]ShortcutFailure, 0, maxShortcutFailures)
	}
	a.shortcutFailures = append([]ShortcutFailure{{
		Time:     time.Now().Format("2006-01-02 15:04:05"),
		Path:     path,
		Reason:   reason,
		Detail:   detail,
		ClientIP: clientIP,
	}}, a.shortcutFailures...)
	if len(a.shortcutFailures) > maxShortcutFailures {
		a.shortcutFailures = a.shortcutFailures[:maxShortcutFailures]
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "shortcut:failure", a.shortcutFailures[0])
	}
}

// GetShortcutFailures returns recent shortcut errors for the desktop UI.
func (a *App) GetShortcutFailures() []ShortcutFailure {
	a.failuresMu.Lock()
	defer a.failuresMu.Unlock()
	out := make([]ShortcutFailure, len(a.shortcutFailures))
	copy(out, a.shortcutFailures)
	return out
}
