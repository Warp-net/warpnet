//nolint:all
package warpnet

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/ipfs/boxo/blockstore"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestNewP2PNode(t *testing.T) {
	node, err := NewP2PNode(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })
	require.NotEmpty(t, node.ID())
}

func TestNewNoise(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := p2pCrypto.UnmarshalEd25519PrivateKey(priv)
	require.NoError(t, err)

	tr, err := NewNoise("/noise", key, nil)
	require.NoError(t, err)
	require.NotNil(t, tr)
}

func TestNewLimiterAndManagers(t *testing.T) {
	limiter := NewConfigurableLimiter(nil)
	require.NotNil(t, limiter)

	fromJSON := NewConfigurableLimiter(strings.NewReader(`{}`))
	require.NotNil(t, fromJSON)

	// malformed config falls back to the defaults rather than failing startup
	fallback := NewConfigurableLimiter(strings.NewReader(`{`))
	require.NotNil(t, fallback)

	cm, err := NewConnManager(limiter)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cm.Close() })

	rm, err := NewResourceManager(limiter)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rm.Close() })

	require.NotNil(t, rm)
}

func TestNewPeerstore(t *testing.T) {
	store, err := NewPeerstore(context.Background(), dssync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })
}

func TestBitswapStack(t *testing.T) {
	node, err := NewP2PNode(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = node.Close() })

	net := NewBitswapNetwork(node)
	require.NotNil(t, net)

	bstore := blockstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	exchange := NewBitswapExchange(context.Background(), net, nil, bstore)
	require.NotNil(t, exchange)
	t.Cleanup(func() { _ = exchange.Close() })

	bserv := NewBlockService(bstore, exchange)
	require.NotNil(t, bserv)
	t.Cleanup(func() { _ = bserv.Close() })

	require.NotNil(t, NewDAGService(bserv))
}

func TestVerifyAuthorship(t *testing.T) {
	local, remote := WarpPeerID("node-local"), WarpPeerID("node-remote")

	t.Run("rejects an unknown author", func(t *testing.T) {
		require.ErrorIs(t, VerifyAuthorship(nil, ""), ErrForeignAuthor)
		require.ErrorIs(t, VerifyAuthorship(nil, remote.String()), ErrForeignAuthor)
		require.ErrorIs(t, VerifyAuthorship(stubStream{}, remote.String()), ErrForeignAuthor)
	})

	t.Run("accepts the sending peer", func(t *testing.T) {
		s := stubStream{conn: stubConn{local: local, remote: remote}}
		require.NoError(t, VerifyAuthorship(s, remote.String()))
	})

	t.Run("rejects a third party", func(t *testing.T) {
		s := stubStream{conn: stubConn{local: local, remote: remote}}
		require.ErrorIs(t, VerifyAuthorship(s, WarpPeerID("someone-else").String()), ErrForeignAuthor)
	})

	t.Run("accepts a paired alias acting as the local node", func(t *testing.T) {
		s := &WarpStreamBody{
			WarpStream:  stubStream{conn: stubConn{local: local, remote: remote}},
			PairedAlias: true,
		}
		require.True(t, s.IsPairedAlias())
		require.NoError(t, VerifyAuthorship(s, local.String()))
	})

	t.Run("rejects an unpaired alias acting as the local node", func(t *testing.T) {
		s := &WarpStreamBody{WarpStream: stubStream{conn: stubConn{local: local, remote: remote}}}
		require.False(t, s.IsPairedAlias())
		require.ErrorIs(t, VerifyAuthorship(s, local.String()), ErrForeignAuthor)
	})
}

type stubConn struct {
	network.Conn
	local, remote WarpPeerID
}

func (c stubConn) LocalPeer() peer.ID  { return c.local }
func (c stubConn) RemotePeer() peer.ID { return c.remote }

type stubStream struct {
	WarpStream
	conn network.Conn
}

func (s stubStream) Conn() network.Conn { return s.conn }
