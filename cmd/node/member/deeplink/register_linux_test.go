//go:build linux

//nolint:all
package deeplink

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXdgAppsDir(t *testing.T) {
	t.Run("honours XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
		dir, err := xdgAppsDir()
		require.NoError(t, err)
		require.Equal(t, "/tmp/xdg-data/applications", dir)
	})

	t.Run("falls back to the home directory", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		dir, err := xdgAppsDir()
		require.NoError(t, err)
		require.Contains(t, dir, filepath.Join(".local", "share", "applications"))
	})
}

func TestRunShortRejectsMissingBinaries(t *testing.T) {
	require.Error(t, runShort("warpnet-no-such-command"))

	_, err := runShortOut("warpnet-no-such-command")
	require.Error(t, err)
}

func TestRunShortRunsAndCaptures(t *testing.T) {
	require.NoError(t, runShort("true"))
	require.Error(t, runShort("false"))

	out, err := runShortOut("echo", "warpnet.desktop")
	require.NoError(t, err)
	require.Contains(t, out, "warpnet.desktop")

	_, err = runShortOut("false")
	require.Error(t, err)
}

// TestRegisterWritesTheDesktopEntry redirects every XDG root at a temp dir so
// the registration never touches the developer's real desktop database.
func TestRegisterWritesTheDesktopEntry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	// xdg-mime is absent on minimal images: the .desktop file is still written,
	// and only the association step fails.
	err := Register()

	desktop := filepath.Join(root, "data", "applications", "warpnet.desktop")
	require.FileExists(t, desktop)

	contents, readErr := os.ReadFile(desktop)
	require.NoError(t, readErr)
	require.Contains(t, string(contents), "x-scheme-handler/"+Scheme)
	require.Contains(t, string(contents), "StartupWMClass=warpnet")

	if err != nil {
		require.Contains(t, err.Error(), "deeplink:")
	}
}

func TestRegisterFailsWhenTheAppsDirCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	// a regular file where the applications directory should go
	blocked := filepath.Join(root, "data")
	require.NoError(t, os.WriteFile(blocked, nil, 0o600))

	t.Setenv("XDG_DATA_HOME", blocked)
	require.Error(t, Register())
}
