package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/WangYihang/Platypus/desktop/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	appName         = "platypus-desktop"
	keychainService = "platypus-desktop"
)

func main() {
	configPath, err := defaultConfigPath()
	if err != nil {
		log.Fatalf("config path: %v", err)
	}

	a, err := app.New(configPath, keychainService)
	if err != nil {
		log.Fatalf("app init: %v", err)
	}

	if err := wails.Run(&options.App{
		Title:  "Platypus Desktop",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        a.Startup,
		DragAndDrop: &options.DragAndDrop{
			// Enable the native OS file-drop handler; Startup() subscribes
			// and re-emits a "files:os-drop" event for the React layer.
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		Bind: []interface{}{
			a,
		},
	}); err != nil {
		log.Fatalf("wails: %v", err)
	}
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName, "profiles.json"), nil
}
