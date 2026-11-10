package main

import (
	"os"
	"path/filepath"
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
