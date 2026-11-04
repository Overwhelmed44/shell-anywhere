package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app, err := NewApp("")
	if err != nil {
		log.Fatal(err)
	}
	if err := wails.Run(&options.App{
		Title:       "Shell Anywhere",
		Width:       1320,
		Height:      860,
		MinWidth:    900,
		MinHeight:   600,
		AssetServer: &assetserver.Options{Assets: assets},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind:        []interface{}{app},
		Linux: &linux.Options{
			Icon: appIcon, ProgramName: "shell-anywhere", WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
