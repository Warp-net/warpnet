package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
