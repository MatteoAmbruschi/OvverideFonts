package registry

import (
	"fmt"

	winreg "golang.org/x/sys/windows/registry"
)

const (
	fontSubsKey     = `Software\Microsoft\Windows NT\CurrentVersion\FontSubstitutes`
	chromePolicyKey = `SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist`
	userFontsKey    = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`
)

// commonFonts are the font names written as substitutes so native apps also use the override.
var commonFonts = []string{
	"Arial", "Arial CE,238", "Arial CYR,204", "Arial Greek,161", "Arial TUR,162",
	"Courier New", "Helvetica", "Times New Roman", "Verdana", "Tahoma",
	"Segoe UI", "Microsoft Sans Serif", "MS Sans Serif", "MS Serif",
}

// ApplyFontSubstitutes writes HKCU FontSubstitutes so native Windows apps use fontName.
func ApplyFontSubstitutes(fontName string) error {
	k, _, err := winreg.CreateKey(winreg.CURRENT_USER, fontSubsKey, winreg.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for _, name := range commonFonts {
		if err := k.SetStringValue(name, fontName); err != nil {
			return err
		}
	}
	return nil
}

// RevertFontSubstitutes removes the override entries written by ApplyFontSubstitutes.
func RevertFontSubstitutes() error {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, fontSubsKey, winreg.SET_VALUE)
	if err != nil {
		return nil // key absent — nothing to revert
	}
	defer k.Close()
	for _, name := range commonFonts {
		_ = k.DeleteValue(name)
	}
	return nil
}

// InstallChromeExtension writes the Chrome enterprise policy that force-installs the extension.
// Writes to HKLM (requires admin; the app manifest requests requireAdministrator).
// updateManifestPath must use forward slashes (file:/// compatible).
func InstallChromeExtension(extID, updateManifestPath string) error {
	k, _, err := winreg.CreateKey(winreg.LOCAL_MACHINE, chromePolicyKey, winreg.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("chrome policy install (HKLM): %w — make sure the app is running as administrator", err)
	}
	defer k.Close()
	return k.SetStringValue("1", extID+";file:///"+updateManifestPath)
}

// InstallUserFont registers a font file in the current user's font table (HKCU, no admin needed).
// fontPath must be the absolute path to the font file.
func InstallUserFont(displayName, fontPath string) error {
	k, _, err := winreg.CreateKey(winreg.CURRENT_USER, userFontsKey, winreg.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(displayName, fontPath)
}

// UninstallUserFont removes a user-font registry entry written by InstallUserFont.
func UninstallUserFont(displayName string) error {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, userFontsKey, winreg.ALL_ACCESS)
	if err != nil {
		return nil
	}
	defer k.Close()
	_ = k.DeleteValue(displayName)
	return nil
}

// UninstallChromeExtension removes the force-install policy from HKLM.
// After calling this, Chrome will remove the extension on its next restart.
func UninstallChromeExtension() error {
	// Delete the forcelist value.
	k, err := winreg.OpenKey(winreg.LOCAL_MACHINE, chromePolicyKey, winreg.ALL_ACCESS)
	if err != nil {
		return nil // already absent
	}
	_ = k.DeleteValue("1")
	k.Close()

	// Walk up and delete empty parent keys (Chrome → Google → Policies).
	for _, path := range []string{
		chromePolicyKey,
		`SOFTWARE\Policies\Google\Chrome`,
		`SOFTWARE\Policies\Google`,
	} {
		pk, err := winreg.OpenKey(winreg.LOCAL_MACHINE, path, winreg.ALL_ACCESS)
		if err != nil {
			continue
		}
		info, err := pk.Stat()
		pk.Close()
		// Only prune if truly empty: no values and no subkeys.
		if err != nil || info.ValueCount > 0 || info.SubKeyCount > 0 {
			break
		}
		_ = winreg.DeleteKey(winreg.LOCAL_MACHINE, path)
	}
	return nil
}
