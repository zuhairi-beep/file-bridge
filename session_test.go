package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientFileKeyStable(t *testing.T) {
	a := ClientFileKey("IMG_1.HEIC", 100, "abc")
	b := ClientFileKey("IMG_1.HEIC", 100, "abc")
	if a != b || a == "" {
		t.Fatalf("unstable key %q %q", a, b)
	}
}

func TestSessionCheckSkipVerified(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionManager()
	sm.SetSaveDir(dir)
	sess := sm.Create("test", 1)
	key := ClientFileKey("a.jpg", 5, "")
	path := dir + "/a.jpg"
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("hello"))
	rec := UploadRecord{FileName: "a.jpg", SavePath: path, ReceivedSize: 5, SHA256: hex.EncodeToString(sum[:])}
	sm.MarkVerified(sess.ID, key, rec)

	results := sm.CheckFiles(sess.ID, []ManifestEntry{{Key: key, Name: "a.jpg", Size: 5, SHA256: rec.SHA256}})
	if len(results) != 1 || !results[0].SkipUpload || results[0].Status != StatusVerified {
		t.Fatalf("check: %+v", results)
	}
	prog := sm.Progress(sess.ID)
	if !prog.ReadyToDelete || prog.Verified != 1 {
		t.Fatalf("progress: %+v", prog)
	}
}

func TestSessionPersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionManager()
	sm.SetSaveDir(dir)
	sess := sm.Create("persist", 2)
	key := ClientFileKey("b.bin", 3, "")
	path := dir + "/b.bin"
	os.WriteFile(path, []byte("xyz"), 0644)
	sum := sha256.Sum256([]byte("xyz"))
	sm.MarkVerified(sess.ID, key, UploadRecord{
		FileName: "b.bin", SavePath: path, ReceivedSize: 3, SHA256: hex.EncodeToString(sum[:]),
	})

	sm2 := NewSessionManager()
	sm2.SetSaveDir(dir)
	prog := sm2.Progress(sess.ID)
	if prog.Verified != 1 {
		t.Fatalf("reload progress: %+v", prog)
	}
}

func TestShortcutSessionAPIResumeAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	app := &App{config: &Config{SaveDir: dir, ShortcutToken: "tok"}, sessions: NewSessionManager()}
	app.sessions.SetSaveDir(dir)
	fsrv := &FileServer{app: app}

	// Create session
	req := httptest.NewRequest("POST", "/api/shortcut/session/create", bytes.NewReader([]byte(`{"label":"run","expectedCount":1}`)))
	req.Header.Set("X-FileBridge-Token", "tok")
	rec := httptest.NewRecorder()
	fsrv.handleShortcutSessionCreate(rec, req)
	var created map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &created)
	sid, _ := created["sessionId"].(string)
	if sid == "" {
		t.Fatalf("create: %s", rec.Body)
	}

	key := ClientFileKey("pic.jpg", 4, "")
	body := []byte("data")
	sum := sha256.Sum256(body)

	// Upload once
	req = httptest.NewRequest("POST", "/api/shortcut/upload?name=pic.jpg", bytes.NewReader(body))
	req.Header.Set("X-FileBridge-Token", "tok")
	req.Header.Set("X-FileBridge-Session", sid)
	req.Header.Set("X-File-Client-Key", key)
	req.Header.Set("X-Expected-Size", "4")
	req.Header.Set("X-Expected-SHA256", hex.EncodeToString(sum[:]))
	rec = httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	var up map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &up)
	if up["ok"] != true {
		t.Fatalf("upload: %v", up)
	}

	// Idempotent second upload (same body)
	req = httptest.NewRequest("POST", "/api/shortcut/upload?name=pic.jpg", bytes.NewReader(body))
	req.Header.Set("X-FileBridge-Token", "tok")
	req.Header.Set("X-FileBridge-Session", sid)
	req.Header.Set("X-File-Client-Key", key)
	rec = httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	json.Unmarshal(rec.Body.Bytes(), &up)
	if up["skipped"] != true {
		t.Fatalf("want skipped, got %v", up)
	}
	if n := countDataFiles(dir); n != 1 {
		t.Fatalf("expected one saved file, got %d", n)
	}

	// Check endpoint skips
	checkBody, _ := json.Marshal(sessionCheckReq{
		SessionID: sid,
		Files:     []ManifestEntry{{Key: key, Name: "pic.jpg", Size: 4, SHA256: hex.EncodeToString(sum[:])}},
	})
	req = httptest.NewRequest("POST", "/api/shortcut/session/check", bytes.NewReader(checkBody))
	req.Header.Set("X-FileBridge-Token", "tok")
	rec = httptest.NewRecorder()
	fsrv.handleShortcutSessionCheck(rec, req)
	var chk map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &chk)
	results, _ := chk["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("check results: %v", chk)
	}
}

func TestShortcutInterruptedLeavesNoFileAndMarksFailed(t *testing.T) {
	dir := t.TempDir()
	app := &App{config: &Config{SaveDir: dir, ShortcutToken: "secret"}, sessions: NewSessionManager()}
	app.sessions.SetSaveDir(dir)
	fsrv := &FileServer{app: app}
	sess := app.sessions.Create("x", 1)
	key := ClientFileKey("cut.MOV", 100, "")

	req := httptest.NewRequest("POST", "/api/shortcut/upload?name=cut.MOV", &failingReader{n: 3})
	req.Header.Set("X-FileBridge-Token", "secret")
	req.Header.Set("X-FileBridge-Session", sess.ID)
	req.Header.Set("X-File-Client-Key", key)
	rec := httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	if rec.Code != 500 {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	if n := countDataFiles(dir); n != 0 {
		t.Fatalf("data files left on disk: %d", n)
	}
	e := app.sessions.getSessionEntry(sess.ID, key)
	if e == nil || e.Status != StatusFailed {
		t.Fatalf("entry: %+v", e)
	}
}

func countDataFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		n++
	}
	return n
}

func TestManifestRegisterAndProgressDenominator(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionManager()
	sm.SetSaveDir(dir)
	sess := sm.Create("batch", 3)
	sm.RegisterFiles(sess.ID, []ManifestEntry{
		{Name: "1.jpg", Size: 1},
		{Name: "2.jpg", Size: 1},
		{Name: "3.jpg", Size: 1},
	})
	p := sm.Progress(sess.ID)
	if p.Registered != 3 || p.ExpectedCount != 3 || p.ReadyToDelete {
		t.Fatalf("progress: %+v", p)
	}
}
