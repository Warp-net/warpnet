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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNode_IsTestnet(t *testing.T) {
	assert.True(t, node{Network: testNetNetwork}.IsTestnet())
	assert.False(t, node{Network: warpnetNetwork}.IsTestnet())
	assert.False(t, node{Network: ""}.IsTestnet())
}

func TestNode_AddrInfos_Empty(t *testing.T) {
	infos, err := node{}.AddrInfos()
	assert.NoError(t, err)
	assert.Empty(t, infos)
}

func TestNode_AddrInfos_Valid(t *testing.T) {
	const (
		addr   = "/ip4/207.154.221.44/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
		peerID = "12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"
	)
	infos, err := node{Bootstrap: []string{addr}}.AddrInfos()
	assert.NoError(t, err)
	assert.Len(t, infos, 1)
	assert.Equal(t, peerID, infos[0].ID.String())
	assert.Len(t, infos[0].Addrs, 1)
}

func TestNode_AddrInfos_Invalid(t *testing.T) {
	_, err := node{Bootstrap: []string{"not-a-multiaddr"}}.AddrInfos()
	assert.Error(t, err)
}

func TestNode_AddrInfos_MissingPeerID(t *testing.T) {
	// A valid multiaddr without a /p2p component cannot yield an AddrInfo.
	_, err := node{Bootstrap: []string{"/ip4/127.0.0.1/tcp/4001"}}.AddrInfos()
	assert.Error(t, err)
}

func TestLogFormat_Constants(t *testing.T) {
	assert.Equal(t, logFormat("json"), JSONFormat)
	assert.Equal(t, logFormat("text"), TextFormat)
}
