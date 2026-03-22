package main

import (
	"embed"
	"fontoverride/internal/installer"
	"fontoverride/internal/server"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

//go:embed assets/extension.crx assets/update_manifest.xml
var extensionAssets embed.FS

//go:embed assets/fonts
var fontAssets embed.FS

func main() {
	srv := server.New(fontAssets)
	ext := installer.New(extensionAssets)
	app := NewApp(srv, ext, fontAssets)

	if err := wails.Run(&options.App{
		Title:     "Font Override",
		Width:     420,
		Height:    560,
		MinWidth:  420,
		MinHeight: 560,
		MaxWidth:  420,
		MaxHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: frontendAssets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	}); err != nil {
		panic(err)
	}
}
