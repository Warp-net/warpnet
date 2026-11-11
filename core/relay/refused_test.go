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

package relay

import (
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type fakeTags struct {
	mx       sync.Mutex
	reserved map[warpnet.WarpPeerID]bool
}

func (f *fakeTags) GetTagInfo(p warpnet.WarpPeerID) *connmgr.TagInfo {
	f.mx.Lock()
	defer f.mx.Unlock()
	if !f.reserved[p] {
		return &connmgr.TagInfo{Tags: map[string]int{}}
	}
	return &connmgr.TagInfo{Tags: map[string]int{reservationTag: 10}}
}

func (f *fakeTags) setReserved(p warpnet.WarpPeerID, isReserved bool) {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.reserved[p] = isReserved
}

func randPeerID(t *testing.T) warpnet.WarpPeerID {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	require.NoError(t, err)
	id, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)
	return id
}

func newTestRefusedPeers(t *testing.T) (*RefusedPeers, *fakeTags, warpnet.WarpPeerID, warpnet.WarpPeerID) {
	t.Helper()
	tags := &fakeTags{reserved: map[warpnet.WarpPeerID]bool{}}
	dest, src := randPeerID(t), randPeerID(t)
	tags.setReserved(dest, true)
	return NewRefusedPeers(tags), tags, dest, src
}

func TestRefusedPeers_RefusesOnlyTheNamedSource(t *testing.T) {
	refused, _, dest, src := newTestRefusedPeers(t)
	other := randPeerID(t)

	require.True(t, refused.AllowConnect(src, nil, dest))
	require.NoError(t, refused.Refuse(dest, src))

	require.False(t, refused.AllowConnect(src, nil, dest))
	require.True(t, refused.AllowConnect(other, nil, dest))
	require.True(t, refused.AllowConnect(src, nil, other))
	require.True(t, refused.AllowReserve(src, nil))
}

func TestRefusedPeers_NeedsAReservation(t *testing.T) {
	refused, _, _, src := newTestRefusedPeers(t)
	stranger := randPeerID(t)

	require.ErrorIs(t, refused.Refuse(stranger, src), ErrNoReservation)
	require.True(t, refused.AllowConnect(src, nil, stranger))
}

func TestRefusedPeers_RefusalExpires(t *testing.T) {
	refused, _, dest, src := newTestRefusedPeers(t)
	refused.ttl = 20 * time.Millisecond

	require.NoError(t, refused.Refuse(dest, src))
	require.False(t, refused.AllowConnect(src, nil, dest))

	require.Eventually(t, func() bool {
		return refused.AllowConnect(src, nil, dest)
	}, time.Second, 10*time.Millisecond)
}

func TestRefusedPeers_DropsDestinationsWithoutReservation(t *testing.T) {
	refused, tags, dest, src := newTestRefusedPeers(t)
	other := randPeerID(t)
	tags.setReserved(other, true)

	require.NoError(t, refused.Refuse(dest, src))
	tags.setReserved(dest, false)
	require.NoError(t, refused.Refuse(other, src))

	require.True(t, refused.AllowConnect(src, nil, dest))
	require.False(t, refused.AllowConnect(src, nil, other))
}

func TestRefusedPeers_LimitsRefusalsPerDestination(t *testing.T) {
	refused, _, dest, src := newTestRefusedPeers(t)

	for range maxRefusedPeers {
		require.NoError(t, refused.Refuse(dest, randPeerID(t)))
	}
	require.ErrorIs(t, refused.Refuse(dest, src), ErrTooManyRefusedPeers)
	require.True(t, refused.AllowConnect(src, nil, dest))
}
