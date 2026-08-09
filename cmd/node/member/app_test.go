package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Warp-net/warpnet/config"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func touchRunLock(t *testing.T, network string) {
	t.Helper()
	dir := config.StoragePath(network)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, local_store.FirstRunLockFile), nil, 0o600))
}

func TestDetectPersistedNetwork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, found := detectPersistedNetwork()
	assert.False(t, found, "a virgin install must have no persisted network")

	touchRunLock(t, "testnet")
	network, found := detectPersistedNetwork()
	require.True(t, found)
	assert.Equal(t, "testnet", network)

	touchRunLock(t, "warpnet")
	network, found = detectPersistedNetwork()
	require.True(t, found)
	assert.Equal(t, "warpnet", network, "warpnet must win when both networks have data")
}
