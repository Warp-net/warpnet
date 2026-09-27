package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Warp-net/warpnet/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferExistingTestnet(t *testing.T) {
	t.Setenv("NODE_NETWORK", "")
	orig := config.Config().Node.Network
	defer config.SetNetwork(orig)

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, orig, "storage")

	preferExistingTestnet(dbPath)
	assert.Equal(t, orig, config.Config().Node.Network, "no testnet database — nothing changes")

	lock := filepath.Join(tmp, "testnet", "storage", "run.lock")
	require.NoError(t, os.MkdirAll(filepath.Dir(lock), 0o750))
	require.NoError(t, os.WriteFile(lock, nil, 0o600))

	preferExistingTestnet(dbPath)
	assert.Equal(t, "testnet", config.Config().Node.Network)

	t.Setenv("NODE_NETWORK", orig)
	config.SetNetwork(orig)
	preferExistingTestnet(dbPath)
	assert.Equal(t, orig, config.Config().Node.Network, "a pinned network must not be overridden")
}

func TestSetLinuxFontConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fontconfig is configured on Linux only")
	}
	t.Setenv("SNAP", "")
	t.Setenv("FONTCONFIG_FILE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "fontconfig.conf")

	require.NoError(t, setLinuxFontConfig(dir))
	assert.Equal(t, path, os.Getenv("FONTCONFIG_FILE"))
	bt, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, fontConfig, string(bt))

	require.NoError(t, setLinuxFontConfig(dir))
	assert.Equal(t, path, os.Getenv("FONTCONFIG_FILE"), "a relaunch keeps its own config")

	t.Setenv("FONTCONFIG_FILE", "/etc/fonts/custom.conf")
	require.NoError(t, setLinuxFontConfig(dir))
	assert.Equal(t, "/etc/fonts/custom.conf", os.Getenv("FONTCONFIG_FILE"), "an explicit config must not be overridden")
}
