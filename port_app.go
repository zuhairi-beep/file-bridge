package main

import (
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetPortSettings returns configured vs active port for the desktop UI.
func (a *App) GetPortSettings() map[string]interface{} {
	return map[string]interface{}{
		"configuredPort": configuredPort(a.config),
		"activePort":     a.fileServer.GetPort(),
		"running":        a.fileServer.IsRunning(),
		"defaultPort":    defaultPort,
		"minPort":        minUserPort,
		"maxPort":        maxUserPort,
	}
}

// CheckPortAvailable reports whether a port can be bound on this machine.
func (a *App) CheckPortAvailable(port int) map[string]interface{} {
	if err := validatePort(port); err != nil {
		return map[string]interface{}{"ok": false, "error": err.Error()}
	}
	free := isLocalPortFree(port)
	return map[string]interface{}{"ok": true, "free": free, "port": port}
}

// ApplyServerPort saves the port, restarts the HTTP server, and refreshes QR URLs.
func (a *App) ApplyServerPort(port int) error {
	if err := validatePort(port); err != nil {
		return err
	}
	if !isLocalPortFree(port) && !(a.fileServer.IsRunning() && a.fileServer.GetPort() == port) {
		return fmt.Errorf("port %d is already in use", port)
	}
	a.config.Port = port
	if err := SaveConfig(a.config); err != nil {
		return err
	}
	if err := a.restartFileServer(); err != nil {
		return err
	}
	a.emitServerChanged()
	return nil
}

// PickFreePortAndApply chooses a free high port, saves it, and restarts the server.
func (a *App) PickFreePortAndApply() (int, error) {
	p, err := findFreeHighPort()
	if err != nil {
		return 0, err
	}
	if err := a.ApplyServerPort(p); err != nil {
		return 0, err
	}
	return p, nil
}

func (a *App) restartFileServer() error {
	if a.fileServer.IsRunning() {
		if err := a.fileServer.Stop(); err != nil {
			return err
		}
	}
	return a.fileServer.Start()
}

func (a *App) emitServerChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "server:changed", a.GetServerInfo())
	}
}

func (a *App) startServerWithPortRecovery() {
	for {
		err := a.fileServer.Start()
		if err == nil {
			a.emitServerChanged()
			return
		}
		log.Printf("Failed to start HTTP server: %v", err)
		var pe *PortStartError
		if e, ok := err.(*PortStartError); ok {
			pe = e
		}
		if a.ctx == nil || pe == nil || (!pe.Busy && !pe.FileBridge) {
			if a.ctx != nil {
				runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
					Type:    runtime.ErrorDialog,
					Title:   "File Bridge",
					Message: err.Error(),
				})
			}
			return
		}
		btn, _ := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:          runtime.QuestionDialog,
			Title:         "Port in use",
			Message:       fmt.Sprintf("Port %d is not available (%s).\n\nPick a free port automatically and update your Shortcut URL?", pe.Port, err.Error()),
			Buttons:       []string{"Use another port", "Cancel"},
			DefaultButton: "Use another port",
			CancelButton:  "Cancel",
		})
		if btn != "Use another port" {
			return
		}
		if _, err := a.PickFreePortAndApply(); err != nil {
			runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   "File Bridge",
				Message: err.Error(),
			})
			return
		}
		return
	}
}
