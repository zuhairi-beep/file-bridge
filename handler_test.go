package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStreamingUploadReturnsChecksum(t *testing.T) {
	dir := t.TempDir()
	app := &App{config: &Config{SaveDir: dir}}
	fsrv := &FileServer{app: app}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	data := map[string][]byte{"a.jpg": bytes.Repeat([]byte("x"), 5<<20), "b.mp4": []byte("hello")}
	for _, n := range []string{"a.jpg", "b.mp4"} {
		w, _ := mw.CreateFormFile("files", n)
		w.Write(data[n])
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	fsrv.handleFileUpload(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res struct{ Files []UploadRecord }
	json.Unmarshal(rec.Body.Bytes(), &res)
	if len(res.Files) != 2 {
		t.Fatalf("got %d files", len(res.Files))
	}
	for _, f := range res.Files {
		want := sha256.Sum256(data[f.FileName])
		if f.SHA256 != hex.EncodeToString(want[:]) || f.ReceivedSize != int64(len(data[f.FileName])) {
			t.Errorf("bad record %+v", f)
		}
		got, _ := os.ReadFile(f.SavePath)
		if !bytes.Equal(got, data[f.FileName]) {
			t.Errorf("content mismatch for %s", f.FileName)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("leftover temp files: %d entries", len(entries))
	}
}

func TestShortcutUpload(t *testing.T) {
	dir := t.TempDir()
	fsrv := &FileServer{app: &App{config: &Config{SaveDir: dir, ShortcutToken: "secret"}}}
	req := httptest.NewRequest("POST", "/api/shortcut/upload?name=IMG_1.HEIC", bytes.NewReader([]byte("photo")))
	rec := httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	if rec.Code != 401 {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
	req = httptest.NewRequest("POST", "/api/shortcut/upload?name=IMG_1.HEIC", bytes.NewReader([]byte("photo")))
	req.Header.Set("X-FileBridge-Token", "secret")
	req.Header.Set("X-Expected-Size", "5")
	rec = httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	var res map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &res)
	sum := sha256.Sum256([]byte("photo"))
	if res["ok"] != true || res["sha256"] != hex.EncodeToString(sum[:]) || res["size"] != float64(5) {
		t.Fatalf("bad response %v", res)
	}
}

type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	f.n--
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestShortcutInterruptedAndMismatchLeaveNoFiles(t *testing.T) {
	dir := t.TempDir()
	fsrv := &FileServer{app: &App{config: &Config{SaveDir: dir, ShortcutToken: "secret"}}}

	req := httptest.NewRequest("POST", "/api/shortcut/upload?name=cut.MOV", &failingReader{n: 3})
	req.Header.Set("X-FileBridge-Token", "secret")
	rec := httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	if rec.Code != 500 {
		t.Fatalf("interrupted upload: want 500, got %d", rec.Code)
	}

	req = httptest.NewRequest("POST", "/api/shortcut/upload?name=short.txt", bytes.NewReader([]byte("hello")))
	req.Header.Set("X-FileBridge-Token", "secret")
	req.Header.Set("X-Expected-Size", "999")
	rec = httptest.NewRecorder()
	fsrv.handleShortcutUpload(rec, req)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"ok":false`)) {
		t.Fatalf("mismatch: want ok:false, got %s", rec.Body)
	}

	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("expected no files left, found %d", len(entries))
	}
}
