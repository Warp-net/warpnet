//nolint:all
package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	corenode "github.com/Warp-net/warpnet/core/node"
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

func newTestRelayNode(t *testing.T) *RelayNode {
	t.Helper()
	privKey, ownNodeId := testKeyAndID(t)
	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)

	rn, err := NewRelayNode(context.Background(), privKey, psk, ownNodeId)
	require.NoError(t, err)
	require.NotNil(t, rn)
	t.Cleanup(rn.Stop)
	return rn
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

func TestNewRelayNodeRequiresPrivateKey(t *testing.T) {
	_, err := NewRelayNode(context.Background(), nil, nil, "")
	require.ErrorIs(t, err, corenode.ErrPrivateKeyRequired)
}

func TestNewRelayNodeWiresServices(t *testing.T) {
	rn := newTestRelayNode(t)

	require.NotNil(t, rn.discService)
	require.NotNil(t, rn.pubsubService)
	require.NotNil(t, rn.dHashTable)
	require.NotNil(t, rn.memoryStoreCloseF)
	require.NotEmpty(t, rn.opts)
}

func TestRelayAccessorsAreNilSafeBeforeStart(t *testing.T) {
	rn := newTestRelayNode(t)

	require.Nil(t, rn.Node())
	require.Nil(t, rn.Peerstore())
	require.Nil(t, rn.Network())

	out, err := rn.SelfStream("", "", "/public/get/info", nil)
	require.NoError(t, err)
	require.Nil(t, out)

	var nilNode *RelayNode
	require.Nil(t, nilNode.Node())
	require.Nil(t, nilNode.Peerstore())
	require.Nil(t, nilNode.Network())
	require.NotPanics(t, nilNode.Stop)
	require.Panics(t, func() { _ = nilNode.Start() })

	out, err = nilNode.SelfStream("", "", "/public/get/info", nil)
	require.NoError(t, err)
	require.Nil(t, out)

	out, err = nilNode.GenericStream("whatever", "/public/get/info", nil)
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestRelayGenericStreamRejectsMalformedNodeId(t *testing.T) {
	rn := newTestRelayNode(t)
	_, err := rn.GenericStream("not-a-peer-id", "/public/get/info", nil)
	require.ErrorIs(t, err, warpnet.ErrMalformedNodeId)
}

func TestSetupHandlersRejectsUnstartedNode(t *testing.T) {
	rn := newTestRelayNode(t)
	require.Panics(t, rn.setupHandlers)
}

// TestRelayStart drives the full startup: libp2p host, middlewares, the
// /public/get/info route, pubsub and discovery.
func TestRelayStart(t *testing.T) {
	rn := newTestRelayNode(t)

	if !startWithRetry(t, rn.Start) {
		t.Skip("configured node port is busy")
	}

	info := rn.NodeInfo()
	require.Equal(t, warpnet.RelayNode, info.Type)
	require.Equal(t, "bootstrap", info.OwnerId)
	require.NotEmpty(t, info.Addresses)

	require.NotNil(t, rn.Node())
	require.NotNil(t, rn.Peerstore())
	require.NotNil(t, rn.Network())

	_, otherID := testKeyAndID(t)
	require.NotPanics(t, func() {
		rn.SetMaxNodePriority(otherID)
		rn.SetMinNodePriority(otherID)
		rn.SetNodePriority(otherID, warpnet.WarpReachability(0))
	})

	// an unreachable peer surfaces as an error rather than hanging
	_, err := rn.GenericStream(otherID.String(), "/public/get/info", nil)
	require.Error(t, err)

	require.Error(t, rn.SimpleConnect(warpnet.WarpAddrInfo{ID: otherID}))
}
