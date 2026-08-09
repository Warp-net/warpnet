package main

import (
	"os"
	"path/filepath"
	"testing"

	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPersistedNetwork(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "warpnet", "storage")

	touchRunLock := func(network string) {
		lock := filepath.Join(tmp, network, "storage", local_store.FirstRunLockFile)
		require.NoError(t, os.MkdirAll(filepath.Dir(lock), 0o750))
		require.NoError(t, os.WriteFile(lock, nil, 0o600))
	}

	assert.Empty(t, detectPersistedNetwork(dbPath), "a virgin install must have no persisted network")

	touchRunLock("testnet")
	assert.Equal(t, "testnet", detectPersistedNetwork(dbPath))

	touchRunLock("warpnet")
	assert.Equal(t, "warpnet", detectPersistedNetwork(dbPath), "warpnet must win when both networks have data")
}
