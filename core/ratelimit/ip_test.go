/*

	Warpnet - Decentralized Social Network
	Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
	<github.com.mecdy@passmail.net>

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package ratelimit

import (
	"fmt"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const peerID = "12D3KooWSjbYrsVoXzJcEtmgJLMVCbPXMzJmNN1JkEZB9LJ2rnmU"

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

func TestIPLimiter_OneFloodingHostNeverShedsTheRest(t *testing.T) {
	rl := NewIPLimiter(Settings{DiscoveryBurst: 2, DiscoveryPerTenSec: 1})
	flooder := addrs(t, "/ip4/1.2.3.4/tcp/4001")

	for range 2 {
		require.True(t, rl.Allow(flooder))
	}
	assert.False(t, rl.Allow(flooder), "the flooding host must run out of budget")

	for i := range 10 {
		other := addrs(t, fmt.Sprintf("/ip4/5.6.7.%d/tcp/4001", i))
		assert.Truef(t, rl.Allow(other), "peer %d must not pay for another host's flood", i)
	}
}

func TestIPLimiter_OneHostIsOneBucket(t *testing.T) {
	rl := NewIPLimiter(Settings{DiscoveryBurst: 2, DiscoveryPerTenSec: 1})

	require.True(t, rl.Allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")))
	require.True(t, rl.Allow(addrs(t, "/ip4/1.2.3.4/tcp/4002")))

	assert.False(t, rl.Allow(addrs(t, "/ip4/1.2.3.4/udp/4003/quic-v1")),
		"every port of one host shares its budget")
	assert.True(t, rl.Allow(addrs(t, "/ip6/2606:4700:4700::1111/tcp/4001")),
		"an IPv6 host is a host of its own")
}

func TestIPLimiter_PeersWithoutAnAddressShareABucket(t *testing.T) {
	rl := NewIPLimiter(Settings{DiscoveryBurst: 2, DiscoveryPerTenSec: 1})

	require.True(t, rl.Allow(nil)) // DHT reports an ID only
	require.True(t, rl.Allow(addrs(t, "/dns4/example.com/tcp/4001")))

	assert.False(t, rl.Allow(nil), "peers of unknown address stay under one budget")
	assert.True(t, rl.Allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")))
}

func TestIPLimiter_ACircuitAddressChargesTheRelay(t *testing.T) {
	rl := NewIPLimiter(Settings{DiscoveryBurst: 1, DiscoveryPerTenSec: 1})
	relayed := "/ip4/1.2.3.4/tcp/4001/p2p/" + peerID + "/p2p-circuit"

	require.True(t, rl.Allow(addrs(t, relayed)))
	assert.False(t, rl.Allow(addrs(t, "/ip4/1.2.3.4/tcp/4001")),
		"a relayed peer is charged to the host that carries it")
}

func TestIPLimiterTakesTheOwnersLimits(t *testing.T) {
	assert.Equal(t, PerTenSeconds(10, 5), NewIPLimiter(Settings{DiscoveryBurst: 10, DiscoveryPerTenSec: 5}).limit)
	assert.Equal(t,
		PerTenSeconds(int64(Defaults.DiscoveryBurst), int64(Defaults.DiscoveryPerTenSec)),
		NewIPLimiter(Settings{}).limit,
	)
}

func TestIPLimiterIsNilSafe(t *testing.T) {
	var limiter *IPLimiter
	require.True(t, limiter.Allow(nil))
	require.NotPanics(t, limiter.Close)
}
