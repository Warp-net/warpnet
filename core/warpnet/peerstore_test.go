//nolint:all
package warpnet

import (
	"context"
	"crypto/rand"
	"testing"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/require"
)

func newTestPeer(t *testing.T) peer.ID {
	_, pub, err := p2pCrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id
}

func TestCachedPeerstore_FollowsProtocolWrites(t *testing.T) {
	store, err := NewPeerstore(context.Background(), dssync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	p := newTestPeer(t)

	require.NoError(t, store.SetProtocols(p, "/a", "/b"))
	supported, err := store.SupportsProtocols(p, "/b", "/c")
	require.NoError(t, err)
	require.Equal(t, []protocol.ID{"/b"}, supported)

	require.NoError(t, store.SetProtocols(p, "/c"))
	first, err := store.FirstSupportedProtocol(p, "/a", "/c")
	require.NoError(t, err)
	require.Equal(t, protocol.ID("/c"), first)

	require.NoError(t, store.AddProtocols(p, "/d"))
	require.NoError(t, store.RemoveProtocols(p, "/c"))
	protocols, err := store.GetProtocols(p)
	require.NoError(t, err)
	require.Equal(t, []protocol.ID{"/d"}, protocols)

	store.RemovePeer(p)
	protocols, err = store.GetProtocols(p)
	require.NoError(t, err)
	require.Empty(t, protocols)
}

func TestCachedPeerstore_KeepsProtocolsAcrossRestarts(t *testing.T) {
	db := dssync.MutexWrap(ds.NewMapDatastore())
	p := newTestPeer(t)

	store, err := NewPeerstore(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, store.SetProtocols(p, "/a"))
	_, ok := peerstore.GetCertifiedAddrBook(store)
	require.True(t, ok)
	require.NoError(t, store.Close())

	reopened, err := NewPeerstore(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	protocols, err := reopened.GetProtocols(p)
	require.NoError(t, err)
	require.Equal(t, []protocol.ID{"/a"}, protocols)
}

func TestCachedPeerstore_AnswersFromMemory(t *testing.T) {
	store, err := NewPeerstore(context.Background(), dssync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	p := newTestPeer(t)
	require.NoError(t, store.SetProtocols(p, "/a", "/b", "/c"))
	_, err = store.SupportsProtocols(p, "/b")
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(100, func() {
		_, _ = store.SupportsProtocols(p, "/b")
	})
	require.LessOrEqual(t, allocs, 2.0)
}
