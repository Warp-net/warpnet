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

package handler

import (
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/warpnet"
	"strings"
	"time"
)

type StatsNodeInformer interface {
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
	Network() warpnet.WarpNetwork
}

type StatsProvider interface {
	Stats() map[string]string
}

func StreamGetStatsHandler(
	i StatsNodeInformer,
	db StatsProvider,
) middleware.WarpHandler {
	return func(_ []byte, s warpnet.WarpStream) (any, error) {
		sent, recv := warpnet.GetNetworkIO()

		networkState := "Disconnected"
		peersOnline := i.Network().Peers()
		if len(peersOnline) != 0 {
			networkState = "Connected"
		}

		storedPeers := i.Peerstore().Peers()
		nodeInfo := i.NodeInfo()

		publicAddrsStr := "Waiting..."
		if len(nodeInfo.Addresses) > 0 {
			publicAddrsStr = strings.Join(nodeInfo.Addresses, ",")
		}

		stats := warpnet.NodeStats{
			UserId:          nodeInfo.OwnerId,
			NodeID:          nodeInfo.ID,
			Version:         nodeInfo.Version,
			PublicAddresses: publicAddrsStr,
			StartTime:       nodeInfo.StartTime.Format(time.DateTime),
			NetworkState:    networkState,
			RelayState:      nodeInfo.RelayState,
			DatabaseStats:   db.Stats(),
			ConsensusStats:  nil,
			MemoryStats:     warpnet.GetMemoryStats(),
			CPUStats:        warpnet.GetCPUStats(),
			BytesSent:       sent,
			BytesReceived:   recv,
			PeersOnline:     len(peersOnline),
			PeersStored:     len(storedPeers),
		}
		return stats, nil
	}
}
