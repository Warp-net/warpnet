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

// registerPlatform writes a per-user .desktop file claiming the
// warpnet:// scheme and asks xdg-mime to make it the default
// handler. No root required.
//
// XDG behaviour:
//   - The .desktop must live somewhere in $XDG_DATA_DIRS or in
//     ~/.local/share/applications.
//   - MimeType=x-scheme-handler/warpnet; declares this app as a
//     candidate handler.
//   - `xdg-mime default warpnet.desktop x-scheme-handler/warpnet`
//     promotes it to the default.
//
// Idempotent: rewriting the file with the same content is a no-op;
// xdg-mime default sets the association (it doesn't refuse if
// already set).
func registerPlatform() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("deeplink: locate own executable: %w", err)
	}

	appsDir, err := xdgAppsDir()
	if err != nil {
		return fmt.Errorf("deeplink: resolve apps dir: %w", err)
	}
	// 0o755 is required by the XDG spec for ~/.local/share/applications:
	// xdg-mime and the various desktop launchers refuse to descend into
	// it otherwise. Not a secret directory; harden via the parent dir
	// permissions ($HOME), not this one.
	if err := os.MkdirAll(appsDir, 0o755); err != nil { //nolint:mnd,gosec // G301
		return fmt.Errorf("deeplink: mkdir %s: %w", appsDir, err)
	}

	desktopPath := filepath.Join(appsDir, "warpnet.desktop")
	// The Icon=warpnet line matches what setLinuxDesktopIcon writes
	// elsewhere in this binary; both functions touch the same file
	// and we want them to produce a consistent final result regardless
	// of which one runs last.
	contents := fmt.Sprintf(
		`[Desktop Entry]
Name=Warpnet
Comment=Decentralized social network
Exec=%s %%u
Icon=warpnet
Terminal=false
Type=Application
Categories=Network;Social;
MimeType=x-scheme-handler/%s;
StartupWMClass=warpnet
`, exe, Scheme)

	// 0o644 — readable by user/group; .desktop launchers must not be
	// world-writable.
	if err := os.WriteFile(desktopPath, []byte(contents), 0o644); err != nil { //nolint:mnd,gosec
		return fmt.Errorf("deeplink: write .desktop: %w", err)
	}

	// Promote to default for the scheme. If xdg-mime isn't installed
	// the desktop entry is still picked up by environments that scan
	// MimeType= directly (GNOME, KDE) — log but don't fail.
	if path, lerr := exec.LookPath("xdg-mime"); lerr == nil {
		// 5s is generous for a metadata write but bounded so a stuck
		// xdg-mime can't hold startup forever. path comes from PATH
		// resolution of the literal "xdg-mime", every other arg is
		// a compile-time constant — no injection surface.
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
