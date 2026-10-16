package main

import (
	"fmt"
	"net/http"
)

func (fs *FileServer) writeShortcutError(w http.ResponseWriter, r *http.Request, status int, reason, detail string) {
	if fs.app != nil {
		fs.app.logShortcutFailure(r.URL.Path, reason, detail, clientIP(r))
	}
	hint := fmt.Sprintf(
		"Check the iOS Shortcut URL in File Bridge. This PC is on port %d; Shortcuts should use port %d unless the app shows another port.",
		fs.port, defaultPort,
	)
	if fs.port == defaultPort {
		hint = "Check the iOS Shortcut URL and X-FileBridge-Token copied from File Bridge on this PC."
	}
	writeJSON(w, status, map[string]interface{}{
		"ok":           false,
		"reason":       reason,
		"error":        detail,
		"port":         fs.port,
		"defaultPort":  defaultPort,
		"shortcutURL":  fs.GetShortcutURL(),
		"uploadURL":    fs.GetUploadURL(),
		"hint":         hint,
	})
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.RemoteAddr
}

func (fs *FileServer) handleShortcutInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"app":          "file-bridge",
		"port":         fs.port,
		"defaultPort":  defaultPort,
		"aliasPorts":   fs.aliasPorts,
		"shortcutURL":  fs.GetShortcutURL(),
		"uploadURL":    fs.GetUploadURL(),
	})
}
