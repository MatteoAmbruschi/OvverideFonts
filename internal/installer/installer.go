package installer

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fontoverride/internal/registry"
)

// ExtensionID is computed from the RSA keypair during the build step.
// Run: go run ./scripts/pack-extension  — it will update this value automatically.
const ExtensionID = "maglimbllfdgmbjklbfjiakndbgdpeem"

type Manager struct {
	assets embed.FS
}

func New(assets embed.FS) *Manager { return &Manager{assets: assets} }

// EnsureInstalled extracts the embedded CRX to %APPDATA%\FontOverride and
// writes the Chrome enterprise policy that force-installs it.
// Chrome must be restarted once after the first install for the policy to apply.
func (m *Manager) EnsureInstalled() error {
	if ExtensionID == "placeholder_run_pack_tool" {
		return fmt.Errorf("run 'go run ./scripts/pack-extension' first to build the Chrome extension")
	}

	dir := filepath.Join(os.Getenv("APPDATA"), "FontOverride")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	crxDst := filepath.Join(dir, "extension.crx")
	manifestDst := filepath.Join(dir, "update_manifest.xml")

	crxData, err := m.assets.ReadFile("assets/extension.crx")
	if err != nil {
		return err
	}
	if err := os.WriteFile(crxDst, crxData, 0o644); err != nil {
		return err
	}

	xmlData, err := m.assets.ReadFile("assets/update_manifest.xml")
	if err != nil {
		return err
	}
	// Replace placeholder with the actual local CRX path (forward-slash for file:/// URL).
	crxURL := filepath.ToSlash(crxDst)
	xml := strings.ReplaceAll(string(xmlData), "CRX_PATH_PLACEHOLDER", crxURL)
	if err := os.WriteFile(manifestDst, []byte(xml), 0o644); err != nil {
		return err
	}

	return registry.InstallChromeExtension(ExtensionID, filepath.ToSlash(manifestDst))
}

// Cleanup removes the Chrome enterprise policy and deletes all files written to
// %APPDATA%\FontOverride. Called on app shutdown so no residual state is left.
func (m *Manager) Cleanup() {
	_ = registry.UninstallChromeExtension()
	dir := filepath.Join(os.Getenv("APPDATA"), "FontOverride")
	_ = os.RemoveAll(dir)
	// Remove stale exe that may appear in Roaming from dev/build operations.
	_ = os.Remove(filepath.Join(os.Getenv("APPDATA"), "FontOverride.exe"))
}
