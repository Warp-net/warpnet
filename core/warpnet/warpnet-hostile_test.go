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
package warpnet

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const knownPeerID = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"

func TestWarpError_IsComparable(t *testing.T) {
	const sentinel WarpError = "boom"

	assert.Equal(t, "boom", sentinel.Error())
	assert.True(t, errors.Is(sentinel, sentinel))

	wrapped := errors.Join(errors.New("context"), sentinel)
	assert.True(t, errors.Is(wrapped, sentinel), "sentinels must survive wrapping")
}

// A handler whose path is malformed must be rejected at registration time —
// a bad route silently swallows every request sent to it.
func TestWarpStreamHandler_IsValid(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/public/get/user/0.0.0", true},
		{"/private/post/tweet/0.0.0", true},
		{"/internal/delete/tweet/0.0.0", true},
		{"public/get/user", false},    // no leading slash
		{"/public/patch/user", false}, // unknown verb
		{"/unknown/get/user", false},  // unknown scope
		{"/", false},                  // no verb, no scope
		{"", false},                   // empty
		{"/get/user", false},          // verb but no scope
		{"/public/user", false},       // scope but no verb
	}

	for _, c := range cases {
		wh := &WarpStreamHandler{Path: WarpProtocolID(c.path)}
		assert.Equalf(t, c.want, wh.IsValid(), "path %q", c.path)
	}
}

func TestWarpStreamHandler_StringNamesThePath(t *testing.T) {
	wh := &WarpStreamHandler{
		Path:    WarpProtocolID("/public/get/user/0.0.0"),
		Handler: func([]byte, WarpStream) (any, error) { return nil, nil },
	}
	assert.Contains(t, wh.String(), "/public/get/user/0.0.0")
}

// Relay detection has to keep working for the two legacy encodings, otherwise
// old bootstrap nodes get treated as ordinary peers and pollute the timeline.
func TestNodeInfo_RoleDetection(t *testing.T) {
	assert.True(t, NodeInfo{Type: RelayNode}.IsRelay())
	assert.True(t, NodeInfo{OwnerId: RelayNode}.IsRelay(), "pre-Type relays marked the role in OwnerId")
	assert.True(t, NodeInfo{OwnerId: "bootstrap"}.IsRelay(), "oldest relays used OwnerId 'bootstrap'")

	assert.False(t, NodeInfo{}.IsRelay())
	assert.False(t, NodeInfo{OwnerId: "some-user-id"}.IsRelay())

	assert.True(t, NodeInfo{Type: ModeratorNode}.IsModerator())
	assert.False(t, NodeInfo{}.IsModerator())
	assert.False(t, NodeInfo{Type: RelayNode}.IsModerator())
}

func TestFromStringToPeerID_RejectsGarbageWithoutPanicking(t *testing.T) {
	valid := FromStringToPeerID(knownPeerID)
	assert.Equal(t, knownPeerID, valid.String())

	for _, bad := range []string{
		"",
		"not-a-peer-id",
		"12D3KooW",
		strings.Repeat("Q", 300),
		"../../etc/passwd",
		"12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j-tampered",
	} {
		assert.Emptyf(t, string(FromStringToPeerID(bad)), "%q must not decode", bad)
	}
}

func TestFromBytesToPeerID_RejectsGarbage(t *testing.T) {
	assert.Empty(t, string(FromBytesToPeerID(nil)))
	assert.Empty(t, string(FromBytesToPeerID([]byte("junk"))))
}

// A peer id round-trips through its public key: this is what lets a node
// verify that a signed gossip message really came from the claimed author.
func TestPeerIDPublicKeyRoundTrip(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	id, err := IDFromPublicKey(pub)
	require.NoError(t, err)
	assert.NotEmpty(t, string(id))

	back := FromIDToPubKey(id)
	assert.Equal(t, ed25519.PublicKey(back), pub, "the id must yield the exact same key")

	pk, err := UnmarshalEd25519PublicKey(pub)
	require.NoError(t, err)
	assert.NotNil(t, pk)
}

func TestIDFromPublicKey_RejectsWrongSizedKeys(t *testing.T) {
	for _, size := range []int{0, 1, 16, 31, 33, 64} {
		_, err := IDFromPublicKey(make([]byte, size))
		assert.Errorf(t, err, "a %d-byte key is not ed25519", size)
	}
}

func TestFromIDToPubKey_UnknownIDYieldsEmpty(t *testing.T) {
	assert.Empty(t, FromIDToPubKey(""), "an empty id has no key to extract")
}

func TestUnmarshalEd25519PublicKey_RejectsGarbage(t *testing.T) {
	_, err := UnmarshalEd25519PublicKey(nil)
	assert.Error(t, err)
	_, err = UnmarshalEd25519PublicKey([]byte("too short"))
	assert.Error(t, err)
}

func TestNewMultiaddr(t *testing.T) {
	a, err := NewMultiaddr("/ip4/1.2.3.4/tcp/4001")
	require.NoError(t, err)
	assert.Equal(t, "/ip4/1.2.3.4/tcp/4001", a.String())

	for _, bad := range []string{"", "not-an-address", "/ip4/999.999.999.999/tcp/4001", "/tcp"} {
		_, err := NewMultiaddr(bad)
		assert.Errorf(t, err, "%q must be rejected", bad)
	}
}

// Announcing a private or loopback address to the DHT is how a node ends up
// unreachable and how peers waste dial attempts.
func TestIsPublicMultiAddress(t *testing.T) {
	public := []string{
		"/ip4/8.8.8.8/tcp/4001",
		"/ip4/207.154.221.44/tcp/4001",
		"/ip6/2606:4700:4700::1111/tcp/4001",
	}
	for _, s := range public {
		a, err := NewMultiaddr(s)
		require.NoError(t, err)
		assert.Truef(t, IsPublicMultiAddress(a), "%s should be public", s)
	}

	private := []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip4/0.0.0.0/tcp/4001",
		"/ip4/10.0.0.5/tcp/4001",
		"/ip4/192.168.1.10/tcp/4001",
		"/ip4/172.16.0.1/tcp/4001",
		"/ip4/169.254.1.1/tcp/4001",
		"/ip4/224.0.0.1/tcp/4001",
		"/ip6/::1/tcp/4001",
	}
	for _, s := range private {
		a, err := NewMultiaddr(s)
		require.NoError(t, err)
		assert.Falsef(t, IsPublicMultiAddress(a), "%s must not be announced as public", s)
	}

	// An address with no IP component at all cannot be judged public.
	dns, err := NewMultiaddr("/dns4/example.com/tcp/4001")
	require.NoError(t, err)
	assert.False(t, IsPublicMultiAddress(dns))
}

func TestRelayAddressDetection(t *testing.T) {
	assert.True(t, IsRelayAddress("/ip4/1.2.3.4/tcp/4001/p2p-circuit/p2p/"+knownPeerID))
	assert.False(t, IsRelayAddress("/ip4/1.2.3.4/tcp/4001"))
	assert.False(t, IsRelayAddress(""))

	circuit, err := NewMultiaddr("/ip4/1.2.3.4/tcp/4001/p2p/" + knownPeerID + "/p2p-circuit")
	require.NoError(t, err)
	assert.True(t, IsRelayMultiaddress(circuit))

	direct, err := NewMultiaddr("/ip4/1.2.3.4/tcp/4001")
	require.NoError(t, err)
	assert.False(t, IsRelayMultiaddress(direct))
}

func TestIsNoAddressesError(t *testing.T) {
	assert.True(t, IsNoAddressesError(routing.ErrNotFound))
	assert.True(t, IsNoAddressesError(errors.Join(errors.New("dial"), routing.ErrNotFound)))
	assert.False(t, IsNoAddressesError(nil))
	assert.False(t, IsNoAddressesError(errors.New("connection refused")))
}

func TestAddrInfoParsing(t *testing.T) {
	full := "/ip4/1.2.3.4/tcp/4001/p2p/" + knownPeerID

	info, err := AddrInfoFromString(full)
	require.NoError(t, err)
	assert.Equal(t, knownPeerID, info.ID.String())
	require.Len(t, info.Addrs, 1)

	maddr, err := NewMultiaddr(full)
	require.NoError(t, err)
	info2, err := AddrInfoFromP2pAddr(maddr)
	require.NoError(t, err)
	assert.Equal(t, info.ID, info2.ID)

	// Missing the /p2p/ component means we have no idea who we'd be dialling.
	_, err = AddrInfoFromString("/ip4/1.2.3.4/tcp/4001")
	assert.Error(t, err)

	_, err = AddrInfoFromString("garbage")
	assert.Error(t, err)
}

func TestHostStatsAreAlwaysPopulated(t *testing.T) {
	// These feed the dashboard; they must never panic or hand back nil maps
	// regardless of what the host OS reports.
	assert.NotPanics(t, func() {
		mem := GetMemoryStats()
		assert.NotNil(t, mem)

		cpu := GetCPUStats()
		assert.NotNil(t, cpu)

		_, _ = GetNetworkIO()
		_ = GetMacAddr()
	})
}

func TestNewConfigurableLimiter_FallsBackOnGarbage(t *testing.T) {
	// A corrupt limits file must not take the node down — it falls back to
	// the built-in defaults.
	assert.NotPanics(t, func() {
		l := NewConfigurableLimiter(strings.NewReader("{ this is not json"))
		assert.NotNil(t, l)
	})

	assert.NotPanics(t, func() {
		l := NewConfigurableLimiter(strings.NewReader("{}"))
		assert.NotNil(t, l)
	})
}
