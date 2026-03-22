package main

import (
	"context"
	"embed"
	"fontoverride/internal/fonts"
	"fontoverride/internal/installer"
	"fontoverride/internal/registry"
	"fontoverride/internal/server"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Status is returned by GetStatus and mirrored as a TypeScript type by Wails.
type Status struct {
	Font   string `json:"font"`
	Active bool   `json:"active"`
}

type App struct {
	ctx        context.Context
	srv        *server.Server
	ext        *installer.Manager
	fontAssets embed.FS
}

func NewApp(srv *server.Server, ext *installer.Manager, fontAssets embed.FS) *App {
	return &App{srv: srv, ext: ext, fontAssets: fontAssets}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.srv.Start()
	if err := a.ext.EnsureInstalled(); err != nil {
		runtime.LogWarningf(a.ctx, "extension install: %v", err)
	}
}

func (a *App) shutdown(_ context.Context) {
	a.srv.Stop()
	_ = registry.RevertFontSubstitutes()
	a.cleanupUserFonts()
	a.ext.Cleanup()
}

// GetFonts merges system-installed fonts with fonts bundled in the binary.
func (a *App) GetFonts() []string {
	system := fonts.List()
	bundled := fonts.ListBundled(a.fontAssets)

	seen := make(map[string]bool, len(system)+len(bundled))
	result := make([]string, 0, len(system)+len(bundled))

	for _, f := range system {
		if !seen[f] {
			seen[f] = true
			result = append(result, f)
		}
	}
	for _, f := range bundled {
		if !seen[f] {
			seen[f] = true
			result = append(result, f)
		}
	}
	sort.Strings(result)
	return result
}

func (a *App) GetStatus() Status {
	font, active := a.srv.GetFont()
	return Status{Font: font, Active: active}
}

func (a *App) ApplyFont(name string) error {
	a.srv.SetFont(name, true)

	// If this is a bundled font, install it into the Windows user font dir so
	// FontSubstitutes (native-app override) can resolve it.
	if a.srv.BundledFontFilename(name) != "" {
		if err := a.installBundledFont(name); err != nil {
			// Non-fatal: Chrome override still works via @font-face served from localhost.
			runtime.LogWarningf(a.ctx, "user-font install: %v", err)
		}
	}

	if err := registry.ApplyFontSubstitutes(name); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "fontChanged", name, true)
	return nil
}

func (a *App) ResetFont() error {
	a.srv.SetFont("", false)
	if err := registry.RevertFontSubstitutes(); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "fontChanged", "", false)
	return nil
}

// installBundledFont extracts a bundled font to the Windows user fonts directory and
// registers it in HKCU so native apps can use it via FontSubstitutes.
func (a *App) installBundledFont(displayName string) error {
	filename := a.srv.BundledFontFilename(displayName)
	if filename == "" {
		return nil
	}
	data, err := a.fontAssets.ReadFile("assets/fonts/" + filename)
	if err != nil {
		return err
	}
	fontDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")
	if err := os.MkdirAll(fontDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(fontDir, filename)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	return registry.InstallUserFont(displayName, dst)
}

// cleanupUserFonts removes bundled fonts we may have installed into the Windows user font dir.
func (a *App) cleanupUserFonts() {
	entries, err := fs.ReadDir(a.fontAssets, "assets/fonts")
	if err != nil {
		return
	}
	fontDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		filename := e.Name()
		switch strings.ToLower(filepath.Ext(filename)) {
		case ".ttf", ".otf", ".woff2", ".woff":
			displayName := fonts.BundledDisplayName(filename)
			_ = registry.UninstallUserFont(displayName)
			_ = os.Remove(filepath.Join(fontDir, filename))
		}
	}
}
