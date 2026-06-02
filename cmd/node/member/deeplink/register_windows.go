//go:build windows

package deeplink

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// HKCU (not HKLM) so we don't need admin; rewritten each launch so a moved .exe stays valid.
func registerPlatform() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("deeplink: locate own executable: %w", err)
	}

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
