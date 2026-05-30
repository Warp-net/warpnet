//go:build linux

package deeplink

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

// Writes ~/.local/share/applications/warpnet.desktop, refreshes the desktop
// MIME cache (update-desktop-database) and asks xdg-mime to make it default.
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
	log.Infof("deeplink: wrote %s (Exec=%q)", desktopPath, exe)

	// Without update-desktop-database the desktop environment's MIME
	// cache keeps the old handler list and ignores the new MimeType=
	// line. Without that, the browser asks the OS "who handles
	// warpnet://?" and gets nothing — clicking the link does nothing.
	if err := runShort("update-desktop-database", appsDir); err != nil {
		log.Warnf("deeplink: update-desktop-database: %v", err)
	}

	if err := runShort("xdg-mime", "default", "warpnet.desktop", "x-scheme-handler/"+Scheme); err != nil {
		log.Warnf("deeplink: xdg-mime default: %v", err)
	}

	// Verify the association actually took. If it didn't, the browser
	// still won't route the scheme — surface that clearly so the user
	// knows what to debug, instead of "nothing happens".
	if out, err := runShortOut("xdg-mime", "query", "default", "x-scheme-handler/"+Scheme); err == nil {
		log.Infof("deeplink: x-scheme-handler/%s now resolves to %q", Scheme, out)
	}
	return nil
}

func runShort(name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not on PATH: %w", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Run() //nolint:gosec // G204
}

func runShortOut(name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not on PATH: %w", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).Output() //nolint:gosec // G204
	if err != nil {
		return "", err
	}
	return string(out), nil
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
