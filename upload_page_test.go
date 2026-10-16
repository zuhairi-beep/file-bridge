package main

import (
	"encoding/json"
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

func TestUploadPageScriptI18nKeysComplete(t *testing.T) {
	used := keysUsedInUploadPageScript()
	for _, lang := range []string{"en", "ja", "ms"} {
		texts := uploadTranslations[lang]
		m := uploadPageJSMap(texts)
		for _, key := range used {
			v, ok := m[key]
			if !ok || strings.TrimSpace(v) == "" {
				t.Fatalf("lang %s missing or empty T.%s (used in script)", lang, key)
			}
		}
	}
}

func TestUploadPageRendersVerifiedLabelEnglish(t *testing.T) {
	fsrv := &FileServer{app: &App{config: &Config{Lang: "en"}}}
	rec := httptest.NewRecorder()
	fsrv.handleUploadPage(rec, httptest.NewRequest("GET", "/upload?lang=en", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"verified":"verified"`) && !strings.Contains(body, `"verified": "verified"`) {
		// compact JSON
		var idx = strings.Index(body, "var T = ")
		if idx < 0 {
			t.Fatal("missing T object")
		}
		end := strings.Index(body[idx:], ";</script>")
		if end < 0 {
			t.Fatal("missing end of T")
		}
		jsonStr := strings.TrimSpace(body[idx+len("var T = ") : idx+end])
		var m map[string]string
		if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
			t.Fatalf("parse T: %v", err)
		}
		if m["verified"] != "verified" || m["uploading"] == "" {
			t.Fatalf("bad T: %+v", m)
		}
	}
}

func TestUploadPageScriptKeysMatchCanonicalList(t *testing.T) {
	used := keysUsedInUploadPageScript()
	if len(used) == 0 {
		t.Fatal("no T. keys found in template")
	}
	for _, k := range used {
		found := false
		for _, c := range uploadPageScriptI18nKeys {
			if c == k {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("script uses T.%s but uploadPageScriptI18nKeys omits it — update uploadPageJS struct", k)
		}
	}
}
