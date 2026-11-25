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

// loggedInTestnet opens a testnet database under appPath and logs into it the
// way the app does, so run.lock lands wherever local-store puts it.
func loggedInTestnet(t *testing.T, appPath string) {
	t.Helper()
	db, err := local_store.New(filepath.Join(appPath, testNetwork, "storage"), local_store.DefaultOptions())
	require.NoError(t, err)
	require.NoError(t, db.Run("tester", "tester-password"))
	db.Close()
}

func TestPreferExistingTestnet(t *testing.T) {
	orig := config.Config().Node.Network
	t.Cleanup(func() { config.SetNetwork(orig) })

	tests := []struct {
		name        string
		pinned      string
		prepare     func(t *testing.T, appPath string)
		wantNetwork string
	}{
		{
			name:        "no testnet database",
			prepare:     func(*testing.T, string) {},
			wantNetwork: mainNetwork,
		},
		{
			name:        "testnet database after a login",
			prepare:     loggedInTestnet,
			wantNetwork: testNetwork,
		},
		{
			name: "testnet folder without a login",
			prepare: func(t *testing.T, appPath string) {
				require.NoError(t, os.MkdirAll(filepath.Join(appPath, testNetwork, "storage", "v0"), 0o750))
			},
			wantNetwork: mainNetwork,
		},
		{
			name:        "pinned network wins over a testnet database",
			pinned:      mainNetwork,
			prepare:     loggedInTestnet,
			wantNetwork: mainNetwork,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NODE_NETWORK", tt.pinned)
			appPath := t.TempDir()
			tt.prepare(t, appPath)
			config.SetNetwork(mainNetwork)

			preferExistingTestnet(filepath.Join(appPath, mainNetwork, "storage"))

			assert.Equal(t, tt.wantNetwork, config.Config().Node.Network)
		})
	}
}

func TestPreferExistingTestnetRepointsDatabase(t *testing.T) {
	t.Setenv("NODE_NETWORK", "")
	orig := config.Config().Node.Network
	t.Cleanup(func() { config.SetNetwork(orig) })

	appPath := t.TempDir()
	loggedInTestnet(t, appPath)
	config.SetNetwork(mainNetwork)
	mainPath := config.Config().Database.Path

	preferExistingTestnet(filepath.Join(appPath, mainNetwork, "storage"))

	assert.Equal(t, filepath.Join(filepath.Dir(filepath.Dir(mainPath)), testNetwork, filepath.Base(mainPath)),
		config.Config().Database.Path)
	assert.NotEmpty(t, config.Config().Node.Bootstrap)
}

func TestPreferExistingTestnetIsIdempotent(t *testing.T) {
	t.Setenv("NODE_NETWORK", "")
	orig := config.Config().Node.Network
	t.Cleanup(func() { config.SetNetwork(orig) })

	appPath := t.TempDir()
	loggedInTestnet(t, appPath)
	config.SetNetwork(testNetwork)
	testPath := config.Config().Database.Path

	preferExistingTestnet(filepath.Join(appPath, testNetwork, "storage"))

	assert.Equal(t, testNetwork, config.Config().Node.Network)
	assert.Equal(t, testPath, config.Config().Database.Path)
}
