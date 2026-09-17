package discovery

import (
	"fmt"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func addrs(t *testing.T, ss ...string) []warpnet.WarpAddress {
	t.Helper()

	out := make([]warpnet.WarpAddress, 0, len(ss))
	for _, s := range ss {
		a, err := warpnet.NewMultiaddr(s)
		require.NoError(t, err)
		out = append(out, a)
	}
	return out
}

func TestIPRateLimiter_OneFloodingHostNeverShedsTheRest(t *testing.T) {
	rl := newIPRateLimiter(2, 1)
	flooder := addrs(t, "/ip4/1.2.3.4/tcp/4001")

	for range 2 {
		require.True(t, rl.allow(flooder))
	}
	assert.False(t, rl.allow(flooder), "the flooding host must run out of budget")

	for i := range 10 {
		other := addrs(t, fmt.Sprintf("/ip4/5.6.7.%d/tcp/4001", i))
		assert.Truef(t, rl.allow(other), "peer %d must not pay for another host's flood", i)
	}
}

func TestIPRateLimiter_OneHostIsOneBucket(t *testing.T) {
	rl := newIPRateLimiter(2, 1)

	require.True(t, rl.allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")))
	require.True(t, rl.allow(addrs(t, "/ip4/1.2.3.4/tcp/4002")))

	assert.False(t, rl.allow(addrs(t, "/ip4/1.2.3.4/udp/4003/quic-v1")),
		"every port of one host shares its budget")
	assert.True(t, rl.allow(addrs(t, "/ip6/2606:4700:4700::1111/tcp/4001")),
		"an IPv6 host is a host of its own")
}

func TestIPRateLimiter_PeersWithoutAnAddressShareABucket(t *testing.T) {
	rl := newIPRateLimiter(2, 1)

	require.True(t, rl.allow(nil)) // DHT reports an ID only
	require.True(t, rl.allow(addrs(t, "/dns4/example.com/tcp/4001")))

	assert.False(t, rl.allow(nil), "peers of unknown address stay under one budget")
	assert.True(t, rl.allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")))
}

func TestIPRateLimiter_ACircuitAddressChargesTheRelay(t *testing.T) {
	rl := newIPRateLimiter(1, 1)
	relayed := "/ip4/1.2.3.4/tcp/4001/p2p/" + peerID + "/p2p-circuit"

	require.True(t, rl.allow(addrs(t, relayed)))
	assert.False(t, rl.allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")),
		"a relayed peer is charged to the host that carries it")
}
