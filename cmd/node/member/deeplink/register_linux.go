//go:build linux

package deeplink

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Writes ~/.local/share/applications/warpnet.desktop and runs xdg-mime default.
func registerPlatform() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("deeplink: locate own executable: %w", err)
	}

	appsDir, err := xdgAppsDir()
	if err != nil {
		return fmt.Errorf("deeplink: resolve apps dir: %w", err)
	}
	// 0o755 required by XDG; harden via $HOME, not this dir.
	if err := os.MkdirAll(appsDir, 0o755); err != nil { //nolint:mnd,gosec // G301
		return fmt.Errorf("deeplink: mkdir %s: %w", appsDir, err)
	}

	// Exec=%q so an install path with spaces survives.
	desktopPath := filepath.Join(appsDir, "warpnet.desktop")
	contents := fmt.Sprintf(
		`[Desktop Entry]
Name=Warpnet
Comment=Decentralized social network
Exec=%q %%u
Icon=warpnet
Terminal=false
Type=Application
Categories=Network;Social;
MimeType=x-scheme-handler/%s;
StartupWMClass=warpnet
`, exe, Scheme)

	if err := os.WriteFile(desktopPath, []byte(contents), 0o644); err != nil { //nolint:mnd,gosec
		return fmt.Errorf("deeplink: write .desktop: %w", err)
	}

	if path, lerr := exec.LookPath("xdg-mime"); lerr == nil {
		// Bounded so a stuck xdg-mime can't hold startup forever.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "default", "warpnet.desktop", "x-scheme-handler/"+Scheme) //nolint:gosec // G204
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("deeplink: xdg-mime default: %w", err)
		}
	}
	return nil
}

func xdgAppsDir() (string, error) {
	if dh := os.Getenv("XDG_DATA_HOME"); dh != "" {
		return filepath.Join(dh, "applications"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "applications"), nil
}
