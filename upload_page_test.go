package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadPageIsLightweightForSafari(t *testing.T) {
	fsrv := &FileServer{app: &App{config: &Config{Lang: "en"}}}
	req := httptest.NewRequest("GET", "/upload?lang=en", nil)
	rec := httptest.NewRecorder()
	fsrv.handleUploadPage(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) > 25000 {
		t.Fatalf("upload HTML too large for mobile Safari: %d bytes", len(body))
	}
	if strings.Contains(body, "0x428a2f98") {
		t.Fatal("SHA-256 constants should not be inlined in upload HTML")
	}
	if !strings.Contains(body, "/upload/sha256.js") {
		t.Fatal("expected lazy sha256.js reference")
	}
	if !strings.Contains(body, "fileInput") {
		t.Fatal("missing upload UI")
	}
}

func TestUploadSha256JSEndpoint(t *testing.T) {
	fsrv := &FileServer{app: &App{}}
	req := httptest.NewRequest("GET", "/upload/sha256.js", nil)
	rec := httptest.NewRecorder()
	fsrv.handleUploadSha256JS(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "sha256File") {
		t.Fatalf("bad sha256.js response: %d", rec.Code)
	}
}
