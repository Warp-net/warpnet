//go:build windows

package deeplink

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// registerPlatform writes per-user registry keys so Windows routes
// warpnet:// URLs to this executable. Per-user (HKCU\Software\Classes)
// rather than HKLM so we don't need admin rights. Idempotent: every
// startup overwrites the keys with the current exe path, which is
// what we want if the user moved the .exe.
func registerPlatform() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("deeplink: locate own executable: %w", err)
	}

	// HKCU\Software\Classes\warpnet
	root, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\`+Scheme,
		registry.WRITE,
	)
	if err != nil {
		return fmt.Errorf("deeplink: create scheme key: %w", err)
	}
	defer root.Close()

	if err := root.SetStringValue("", "URL:Warpnet Protocol"); err != nil {
		return fmt.Errorf("deeplink: set scheme description: %w", err)
	}
	if err := root.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Errorf("deeplink: set URL Protocol marker: %w", err)
	}

	// HKCU\Software\Classes\warpnet\shell\open\command\(Default) =
	// "C:\path\to\warpnet.exe" "%1"
	cmdKey, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\`+Scheme+`\shell\open\command`,
		registry.WRITE,
	)
	if err != nil {
		return fmt.Errorf("deeplink: create command key: %w", err)
	}
	defer cmdKey.Close()

	value := fmt.Sprintf(`"%s" "%%1"`, exe)
	if err := cmdKey.SetStringValue("", value); err != nil {
		return fmt.Errorf("deeplink: set command: %w", err)
	}
	return nil
}
