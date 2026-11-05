package node

import (
	"context"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	corenode "github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/stretchr/testify/require"
)

func TestMemberBlocklistIsWiredAtStartup(t *testing.T) {
	m, _, _ := newTestMemberNode(t)
	m.opts = append(m.opts, libp2p.NoListenAddrs, libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, m.Start())
	psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
	require.NoError(t, err)
	remoteOpts := append([]libp2p.Option{}, corenode.CommonOptions...)
	remoteOpts = append(remoteOpts,
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
	)
	remote, err := libp2p.New(remoteOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = remote.Close() })
	info := peer.AddrInfo{ID: remote.ID(), Addrs: remote.Addrs()}
	require.NoError(t, m.SimpleConnect(info))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	relayInfo := peer.AddrInfo{ID: m.Node().ID(), Addrs: m.Node().Addrs()}
	_, err = relayclient.Reserve(ctx, remote, relayInfo)
	require.NoError(t, err, "reservation must work before blocking")
	require.NoError(t, m.nodeRepo.BlocklistPermanent(remote.ID().String()))
	require.Empty(t, m.Network().ConnsToPeer(remote.ID()))
	require.Error(t, m.SimpleConnect(info))
	_, err = relayclient.Reserve(ctx, remote, relayInfo)
	require.Error(t, err, "blocked peers cannot renew a relay reservation")
	require.NoError(t, m.nodeRepo.BlocklistRemove(remote.ID().String()))
	require.NoError(t, m.SimpleConnect(info))
}

func TestMemberBlocklistDisconnectsAndRejectsBothDirections(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		name := "temporary"
		if permanent {
			name = "permanent"
		}
		t.Run(name, func(t *testing.T) {
			b := &memberBlocklist{NodeRepo: database.NewNodeRepo(testDB(t))}
			member, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.ConnectionGater(b))
			require.NoError(t, err)
			t.Cleanup(func() { _ = member.Close() })
			remote, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = remote.Close() })
			b.onBlock = func(id peer.ID) { require.NoError(t, member.Network().ClosePeer(id)) }
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			remoteInfo := peer.AddrInfo{ID: remote.ID(), Addrs: remote.Addrs()}
			require.NoError(t, member.Connect(ctx, remoteInfo))
			const path protocol.ID = "/test/blocklist/1"
			incoming := make(chan network.Stream, 1)
			member.SetStreamHandler(path, func(s network.Stream) { incoming <- s })
			s, err := remote.NewStream(ctx, member.ID(), path)
			require.NoError(t, err)
			_, err = s.Write([]byte("open"))
			require.NoError(t, err)
			select {
			case accepted := <-incoming:
				defer accepted.Close()
			case <-ctx.Done():
				t.Fatal("stream was not accepted before blocking")
			}
			if permanent {
				require.NoError(t, b.BlocklistPermanent(remote.ID().String()))
			} else {
				require.NoError(t, b.BlocklistExponential(remote.ID().String()))
			}
			require.Eventually(t, func() bool {
				return len(member.Network().ConnsToPeer(remote.ID())) == 0 && len(remote.Network().ConnsToPeer(member.ID())) == 0
			}, time.Second, time.Millisecond*10)
			_, err = s.Read(make([]byte, 1))
			require.Error(t, err, "existing streams must close")
			require.Error(t, member.Connect(ctx, remoteInfo), "outgoing dial must fail")
			// The initiator may briefly finish its side of the security handshake;
			// opening an application stream must still fail on the blocked receiver.
			_, err = remote.NewStream(ctx, member.ID(), path)
			require.Error(t, err, "incoming application handshake must not run")
			require.NoError(t, b.BlocklistRemove(remote.ID().String()))
			require.NoError(t, member.Connect(ctx, remoteInfo), "unblocking must allow reconnection")
		})
	}
}
