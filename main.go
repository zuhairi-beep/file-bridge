package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	headless := flag.Bool("headless", false, "run only the upload server, without the desktop window")
	dir := flag.String("dir", "", "save directory (headless mode)")
	port := flag.Int("port", 0, "HTTP port (headless mode, default from config or 48765)")
	token := flag.String("token", "", "iOS Shortcut token (headless mode, default from config)")
	flag.Parse()

	release, err := EnsureSingleInstance()
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			fmt.Println("File Bridge is already running. Check the taskbar or close the other copy in Task Manager.")
		} else {
			fmt.Println(err.Error())
		}
		os.Exit(1)
	}
	defer release()

	app := NewApp()

	if *headless {
		runHeadless(app, *dir, *port, *token)
		return
	}

	err = wails.Run(&options.App{
		Title:  "File Bridge",
		Width:  560,
		Height: 820,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// runHeadless starts just the HTTP upload server (useful for servers, testing
// and CI) and blocks until interrupted.
func runHeadless(app *App, dir string, port int, token string) {
	if dir != "" {
		app.config.SaveDir = dir
	}
	if port != 0 {
		app.config.Port = port
	}
	if token != "" {
		app.config.ShortcutToken = token
	}
	os.MkdirAll(app.config.SaveDir, 0755)
	if err := app.fileServer.Start(); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
	log.Printf("Headless mode. Save dir: %s", app.config.SaveDir)
	log.Printf("Phone page: %s", app.fileServer.GetUploadURL())
	log.Printf("Shortcut URL: %s (token %s)", app.fileServer.GetShortcutURL(), app.config.ShortcutToken)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	app.fileServer.Stop()
}
