package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShortcutErrorIncludesPortHint(t *testing.T) {
	app := &App{config: &Config{ShortcutToken: "t"}, sessions: NewSessionManager()}
	fsrv := &FileServer{app: app, port: 48765, lanIP: "127.0.0.1", running: true}
	req := httptest.NewRequest("POST", "/api/shortcut/upload", nil)
	rec := httptest.NewRecorder()
	fsrv.writeShortcutError(rec, req, http.StatusUnauthorized, "401 unauthorized", "invalid token")
	var res map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res["defaultPort"] != float64(defaultPort) || res["shortcutURL"] == "" || res["hint"] == "" {
		t.Fatalf("response: %v", res)
	}
	if len(app.GetShortcutFailures()) != 1 {
		t.Fatalf("expected failure log")
	}
}

func TestShortcutInfoEndpoint(t *testing.T) {
	fsrv := &FileServer{app: &App{config: &Config{Lang: "en"}}, port: 48765, lanIP: "192.168.1.5", running: true}
	req := httptest.NewRequest("GET", "/api/shortcut/info", nil)
	rec := httptest.NewRecorder()
	fsrv.handleShortcutInfo(rec, req)
	var res map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res["app"] != "file-bridge" || res["port"] != float64(48765) {
		t.Fatalf("%v", res)
	}
}
