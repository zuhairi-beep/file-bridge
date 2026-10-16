package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestValidatePort(t *testing.T) {
	if validatePort(1023) == nil || validatePort(65536) == nil {
		t.Fatal("out of range should fail")
	}
	if validatePort(48765) != nil {
		t.Fatal("48765 should be valid")
	}
}

func TestFindFreeHighPort(t *testing.T) {
	p, err := findFreeHighPort()
	if err != nil || p < highPortScanStart {
		t.Fatalf("findFreeHighPort: %v %d", err, p)
	}
	if !isLocalPortFree(p) {
		t.Fatalf("reported free port %d is not free", p)
	}
}

func TestApplyServerPortSavesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{SaveDir: dir, Port: 48765, ShortcutToken: "x", Lang: "en", AllowEphemeralPort: true}
	app := &App{config: cfg, sessions: NewSessionManager()}
	app.fileServer = NewFileServer(app)
	if err := app.fileServer.Start(); err != nil {
		t.Fatal(err)
	}
	defer app.fileServer.Stop()

	p, err := findFreeHighPort()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.ApplyServerPort(p); err != nil {
		t.Fatal(err)
	}
	if app.config.Port != p || app.fileServer.GetPort() != p {
		t.Fatalf("port not applied: cfg=%d active=%d", app.config.Port, app.fileServer.GetPort())
	}
}

func TestPortStartErrorBusy(t *testing.T) {
	ip, err := getLANIP()
	if err != nil {
		t.Skip("no LAN IP")
	}
	ln, err := net.Listen("tcp4", fmt.Sprintf("%s:48765", ip))
	if err != nil {
		t.Skip("cannot occupy 48765")
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ln.Close()

	app := &App{config: &Config{SaveDir: t.TempDir(), Port: 48765, ShortcutToken: "x"}}
	fs := NewFileServer(app)
	err = fs.Start()
	if _, ok := err.(*PortStartError); !ok {
		t.Fatalf("want PortStartError, got %v", err)
	}
}
