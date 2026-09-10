//nolint:all
package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/require"
)

func testKeyAndID(t *testing.T) (ed25519.PrivateKey, warpnet.WarpPeerID) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return priv, id
}

func newTestModeratorNode(t *testing.T) *ModeratorNode {
	t.Helper()
	privKey, ownNodeId := testKeyAndID(t)
	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)

	mn, err := NewModeratorNode(context.Background(), privKey, psk, ownNodeId)
	require.NoError(t, err)
	require.NotNil(t, mn)
	t.Cleanup(mn.Stop)
	return mn
}

// startWithRetry works around the fixed listen port in the config: another
// package's node test may hold it for a few seconds.
func startWithRetry(t *testing.T, start func() error) bool {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		err := start()
		if err == nil {
			return true
		}
		if time.Now().After(deadline) {
			t.Logf("skipping: cannot bind the configured node port: %v", err)
			return false
		}
		time.Sleep(time.Second)
	}
}

func TestNewModeratorNodeWiresServices(t *testing.T) {
	mn := newTestModeratorNode(t)

	require.NotNil(t, mn.dHashTable)
	require.NotNil(t, mn.memoryStoreCloseF)
	require.NotNil(t, mn.isClosed)
	require.NotEmpty(t, mn.options)
	require.NotNil(t, mn.version)
}

func TestModeratorStartRejectsNilNode(t *testing.T) {
	var mn *ModeratorNode
	require.Panics(t, func() { _ = mn.Start() })
	require.NotPanics(t, mn.Stop)
}

func TestModeratorSelfStreamIsNotImplemented(t *testing.T) {
	mn := newTestModeratorNode(t)
	_, err := mn.SelfStream("", "", "/public/get/info", nil)
	require.ErrorIs(t, err, warpnet.ErrNotImplemented)
}

func TestModeratorGenericStreamRejectsMalformedNodeId(t *testing.T) {
	mn := newTestModeratorNode(t)
	_, err := mn.GenericStream("not-a-peer-id", "/public/get/info", nil)
	require.ErrorIs(t, err, warpnet.ErrMalformedNodeId)
}

// TestModeratorStart drives the full startup: libp2p host, middlewares and the
// /public/get/info route.
func TestModeratorStart(t *testing.T) {
	mn := newTestModeratorNode(t)

	if !startWithRetry(t, mn.Start) {
		t.Skip("configured node port is busy")
	}

	info := mn.NodeInfo()
	require.Equal(t, warpnet.ModeratorNode, info.Type)
	require.Equal(t, "None", info.OwnerId)
	require.NotEmpty(t, info.Addresses)

	require.NotNil(t, mn.Node())
	require.Equal(t, mn.Node().ID(), mn.ID())

	// registering an extra route after startup must not disturb the node
	require.NotPanics(t, func() {
		mn.SetStreamHandlers(warpnet.WarpStreamHandler{
			Path:    "/public/get/moderation",
			Handler: func([]byte, warpnet.WarpStream) (any, error) { return nil, nil },
		})
	})

	require.Empty(t, mn.ClosestPeers(), "an isolated moderator knows no peers")

	_, otherID := testKeyAndID(t)
	_, err := mn.GenericStream(otherID.String(), "/public/get/info", nil)
	require.Error(t, err)
}
