package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fastAPILike mimics the program the user had on the preferred port.
func fastAPILike() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"detail":"Not Found"}`))
	})
}

func TestSelfCheckRejectsOtherProgram(t *testing.T) {
	other := httptest.NewServer(fastAPILike())
	defer other.Close()
	if err := selfCheck(other.URL+"/api/ping", "abc"); err == nil {
		t.Fatal("self-check accepted a foreign server")
	}
}

func TestStartFallsBackWhenPortTakenAndQRServesUploadPage(t *testing.T) {
	ip, err := getLANIP()
	if err != nil {
		t.Skip("no LAN IP in this environment")
	}
	// Occupy a port with a foreign HTTP server
	ln, err := net.Listen("tcp4", ip+":0")
	if err != nil {
		t.Fatal(err)
	}
	busy := ln.Addr().(*net.TCPAddr).Port
	go http.Serve(ln, fastAPILike())
	defer ln.Close()

	app := &App{config: &Config{SaveDir: t.TempDir(), Port: busy, ShortcutToken: "x", Lang: "en"}}
	fs := NewFileServer(app)
	app.fileServer = fs
	if err := fs.Start(); err != nil {
		t.Fatal(err)
	}
	defer fs.Stop()

	if fs.GetPort() == busy {
		t.Fatalf("kept the busy port %d", busy)
	}
	if !strings.Contains(fs.GetPortWarning(), fmt.Sprint(busy)) {
		t.Errorf("missing warning, got %q", fs.GetPortWarning())
	}
	// Exactly the URL the QR code encodes must serve the upload page
	qr := fs.GetUploadURL()
	if !strings.Contains(qr, fmt.Sprintf(":%d/upload", fs.GetPort())) {
		t.Fatalf("QR URL %s does not use the real port", qr)
	}
	for _, u := range []string{qr, fmt.Sprintf("http://%s:%d/", ip, fs.GetPort())} {
		resp, err := http.Get(u)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("GET %s: %v %v", u, err, resp)
		}
		resp.Body.Close()
	}
}
