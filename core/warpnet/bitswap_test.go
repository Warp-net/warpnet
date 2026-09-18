//nolint:all
package warpnet

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/boxo/bitswap/network/bsnet"
	"github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// A node runs a bitswap stack per CRDT store (stats and rating) over one
// libp2p host. Starting a stack registers the bitswap protocols with
// host.SetStreamHandler, so without a prefix of its own the stack that
// starts second takes the handlers away and the first one stops serving
// blocks - which the DAG syncer only ever sees as a fetch timeout.
func TestBitswapPrefixKeepsTwoStacksOnOneHostServing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	newStack := func(h P2PNode, opts ...bsnet.NetOpt) blockstore.Blockstore {
		bstore := blockstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
		ex := NewBitswapExchange(ctx, NewBitswapNetwork(h, opts...), nil, bstore)
		t.Cleanup(func() { _ = ex.Close() })
		return bstore
	}

	for _, tc := range []struct {
		name      string
		stats     []bsnet.NetOpt
		rating    []bsnet.NetOpt
		statsSeen bool
	}{
		{name: "shared protocols lose the first stack"},
		{
			name:      "a prefix per stack keeps both",
			stats:     []bsnet.NetOpt{BitswapPrefix("/warpnet/stats")},
			rating:    []bsnet.NetOpt{BitswapPrefix("/warpnet/rating")},
			statsSeen: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, err := NewP2PNode(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = server.Close() })

			// same order as the member node: stats first, rating second
			statsStore := newStack(server, tc.stats...)
			ratingStore := newStack(server, tc.rating...)

			statsBlock := blocks.NewBlock([]byte("stats delta"))
			ratingBlock := blocks.NewBlock([]byte("rating delta"))
			require.NoError(t, statsStore.Put(ctx, statsBlock))
			require.NoError(t, ratingStore.Put(ctx, ratingBlock))

			client, err := NewP2PNode(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			require.NoError(t, client.Connect(ctx, peer.AddrInfo{
				ID: server.ID(), Addrs: server.Addrs(),
			}))

			fetch := func(opts []bsnet.NetOpt, want blocks.Block) error {
				bstore := blockstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
				ex := NewBitswapExchange(ctx, NewBitswapNetwork(client, opts...), nil, bstore)
				defer func() { _ = ex.Close() }()
				ex.PeerConnected(server.ID()) // the connection predates this exchange

				fetchCtx, done := context.WithTimeout(ctx, 5*time.Second)
				defer done()
				_, err := ex.GetBlock(fetchCtx, want.Cid())
				return err
			}

			// the rating stack starts last, so it answers either way
			require.NoError(t, fetch(tc.rating, ratingBlock), "rating block")

			err = fetch(tc.stats, statsBlock)
			if tc.statsSeen {
				require.NoError(t, err, "stats block")
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded, "stats block")
			}
		})
	}
}
