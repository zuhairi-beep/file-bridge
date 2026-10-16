package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type sessionCreateReq struct {
	Label         string `json:"label"`
	ExpectedCount int    `json:"expectedCount"`
}

type sessionRegisterReq struct {
	SessionID string          `json:"sessionId"`
	Files     []ManifestEntry `json:"files"`
}

type sessionCheckReq struct {
	SessionID string          `json:"sessionId"`
	Files     []ManifestEntry `json:"files"`
}

type sessionFailReq struct {
	SessionID string `json:"sessionId"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Reason    string `json:"reason"`
}

func (fs *FileServer) ensureSessions() {
	if fs.app.sessions == nil {
		fs.app.sessions = NewSessionManager()
	}
	fs.app.sessions.SetSaveDir(fs.app.GetSaveDir())
}

func (fs *FileServer) shortcutAuth(r *http.Request) bool {
	token := fs.app.config.ShortcutToken
	got := r.Header.Get("X-FileBridge-Token")
	return token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func (fs *FileServer) handleShortcutSessionCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !fs.shortcutAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "invalid token"})
		return
	}
	saveDir := fs.app.GetSaveDir()
	if saveDir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "save directory unavailable"})
		return
	}
	fs.ensureSessions()

	var req sessionCreateReq
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if strings.TrimSpace(req.Label) == "" {
		req.Label = "iPhone backup"
	}
	sess := fs.app.sessions.Create(req.Label, req.ExpectedCount)
	base := fmt.Sprintf("http://%s:%d", fs.lanIP, fs.port)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"sessionId":   sess.ID,
		"label":       sess.Label,
		"progressURL": base + "/api/shortcut/session/progress?sessionId=" + sess.ID,
		"checkURL":    base + "/api/shortcut/session/check",
		"registerURL": base + "/api/shortcut/session/register",
		"phoneURL":    base + "/shortcut/session?sessionId=" + sess.ID,
	})
}

func (fs *FileServer) handleShortcutSessionRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !fs.shortcutAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "invalid token"})
		return
	}
	fs.ensureSessions()
	var req sessionRegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid body"})
		return
	}
	fs.app.sessions.RegisterFiles(req.SessionID, req.Files)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "progress": fs.app.sessions.Progress(req.SessionID)})
}

func (fs *FileServer) handleShortcutSessionCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !fs.shortcutAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "invalid token"})
		return
	}
	fs.ensureSessions()
	var req sessionCheckReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid body"})
		return
	}
	results := fs.app.sessions.CheckFiles(req.SessionID, req.Files)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"results":  results,
		"progress": fs.app.sessions.Progress(req.SessionID),
	})
}

func (fs *FileServer) handleShortcutSessionProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Progress can be read without token on LAN (session id is unguessable). Shortcut still sends token when available.
	fs.ensureSessions()
	id := r.URL.Query().Get("sessionId")
	if id == "" {
		id = fs.app.sessions.ActiveSessionID()
	}
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "no session"})
		return
	}
	prog := fs.app.sessions.Progress(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "progress": prog})
}

func (fs *FileServer) handleShortcutSessionFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !fs.shortcutAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"ok": false, "error": "invalid token"})
		return
	}
	fs.ensureSessions()
	var req sessionFailReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid body"})
		return
	}
	key := req.Key
	if key == "" {
		key = ClientFileKey(req.Name, req.Size, req.SHA256)
	}
	fs.app.sessions.MarkFailed(req.SessionID, key, req.Reason)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "progress": fs.app.sessions.Progress(req.SessionID)})
}

func (fs *FileServer) handleShortcutSessionPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("sessionId")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, shortcutSessionPageHTML(id))
}
