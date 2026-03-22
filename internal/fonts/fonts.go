package fonts

import (
	"embed"
	"path/filepath"
	"sort"
	"strings"

	winreg "golang.org/x/sys/windows/registry"
)

// List returns all installed font family names from Windows registry, sorted alphabetically.
func List() []string {
	k, err := winreg.OpenKey(winreg.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`,
		winreg.READ)
	if err != nil {
		return nil
	}
	defer k.Close()

	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	result := make([]string, 0, len(names))
	for _, name := range names {
		if clean := stripRegistrySuffix(name); clean != "" && !seen[clean] {
			seen[clean] = true
			result = append(result, clean)
		}
	}
	sort.Strings(result)
	return result
}

// ListBundled returns display names of font files embedded in the given FS (assets/fonts/).
func ListBundled(fsys embed.FS) []string {
	entries, err := fsys.ReadDir("assets/fonts")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isFontExt(filepath.Ext(name)) {
			names = append(names, BundledDisplayName(name))
		}
	}
	return names
}

// BundledDisplayName converts a font filename to a display name.
// "OpenDyslexic-Regular.otf" → "OpenDyslexic"
func BundledDisplayName(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, suffix := range []string{
		"-Regular", "_Regular", " Regular",
		"-Bold", "-Italic", "-Light", "-Medium", "-SemiBold", "-Thin",
	} {
		if strings.HasSuffix(name, suffix) {
			name = name[:len(name)-len(suffix)]
			break
		}
	}
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.TrimSpace(name)
}

func isFontExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".ttf", ".otf", ".woff2", ".woff":
		return true
	}
	return false
}

// stripRegistrySuffix removes "(TrueType)", "(OpenType)" etc. from font registry names.
func stripRegistrySuffix(name string) string {
	if i := strings.Index(name, " ("); i > 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name)
}
