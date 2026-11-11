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
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/require"
)

type fakeTags struct {
	mx       sync.Mutex
	reserved map[warpnet.WarpPeerID]bool
}

func (f *fakeTags) GetTagInfo(p warpnet.WarpPeerID) *warpnet.WarpTagInfo {
	f.mx.Lock()
	defer f.mx.Unlock()
	if !f.reserved[p] {
		return &warpnet.WarpTagInfo{Tags: map[string]int{}}
	}
	return &warpnet.WarpTagInfo{Tags: map[string]int{reservationTag: 10}}
}

func (f *fakeTags) setReserved(p warpnet.WarpPeerID, isReserved bool) {
	f.mx.Lock()
	defer f.mx.Unlock()
	f.reserved[p] = isReserved
}

func randPeerID(t *testing.T) warpnet.WarpPeerID {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id
}

func newTestReservationRefusals(t *testing.T) (*ReservationRefusals, *fakeTags, warpnet.WarpPeerID, warpnet.WarpPeerID) {
	t.Helper()
	tags := &fakeTags{reserved: map[warpnet.WarpPeerID]bool{}}
	dest, src := randPeerID(t), randPeerID(t)
	tags.setReserved(dest, true)
	return NewReservationRefusals(tags), tags, dest, src
}

func TestReservationRefusals_RefusesOnlyTheNamedSource(t *testing.T) {
	refusals, _, dest, src := newTestReservationRefusals(t)
	other := randPeerID(t)

	require.True(t, refusals.AllowConnect(src, nil, dest))
	require.NoError(t, refusals.RefuseReservation(dest, src))

	require.False(t, refusals.AllowConnect(src, nil, dest))
	require.True(t, refusals.AllowConnect(other, nil, dest))
	require.True(t, refusals.AllowConnect(src, nil, other))
	require.True(t, refusals.AllowReserve(src, nil))
}

func TestReservationRefusals_NeedsAReservation(t *testing.T) {
	refusals, _, _, src := newTestReservationRefusals(t)
	stranger := randPeerID(t)

	require.ErrorIs(t, refusals.RefuseReservation(stranger, src), ErrNoReservation)
	require.True(t, refusals.AllowConnect(src, nil, stranger))
}

func TestReservationRefusals_RefusalExpires(t *testing.T) {
	refusals, _, dest, src := newTestReservationRefusals(t)
	refusals.ttl = 20 * time.Millisecond

	require.NoError(t, refusals.RefuseReservation(dest, src))
	require.False(t, refusals.AllowConnect(src, nil, dest))

	require.Eventually(t, func() bool {
		return refusals.AllowConnect(src, nil, dest)
	}, time.Second, 10*time.Millisecond)
}

func TestReservationRefusals_DropsDestinationsWithoutReservation(t *testing.T) {
	refusals, tags, dest, src := newTestReservationRefusals(t)
	other := randPeerID(t)
	tags.setReserved(other, true)

	require.NoError(t, refusals.RefuseReservation(dest, src))
	tags.setReserved(dest, false)
	require.NoError(t, refusals.RefuseReservation(other, src))

	require.True(t, refusals.AllowConnect(src, nil, dest))
	require.False(t, refusals.AllowConnect(src, nil, other))
}

func TestReservationRefusals_LimitsRefusalsPerDestination(t *testing.T) {
	refusals, _, dest, src := newTestReservationRefusals(t)

	for range maxReservationRefusals {
		require.NoError(t, refusals.RefuseReservation(dest, randPeerID(t)))
	}
	require.ErrorIs(t, refusals.RefuseReservation(dest, src), ErrTooManyReservationRefusals)
	require.True(t, refusals.AllowConnect(src, nil, dest))
}
