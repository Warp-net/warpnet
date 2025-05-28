/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package handler

import (
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type NodeInformer interface {
	NodeInfo() warpnet.NodeInfo
	SimpleConnect(warpnet.WarpAddrInfo) error
}

func StreamGetInfoHandler(
	i NodeInformer,
	handler discovery.DiscoveryHandler,
) warpnet.WarpStreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() { s.Close() }() //#nosec

		remoteID := s.Conn().RemotePeer()
		remoteAddr := s.Conn().RemoteMultiaddr()

		info := warpnet.WarpAddrInfo{
			ID:    remoteID,
			Addrs: []warpnet.WarpAddress{remoteAddr},
		}

		log.Debugf("node info request received: %s %s", remoteID, remoteAddr)

		if handler != nil {
			handler(info)
		}

		if err := json.JSON.NewEncoder(s).Encode(i.NodeInfo()); err != nil {
			log.Errorf("fail encoding generic response: %v", err)
		}

		go backConnect(i, info)
		return
	}
}

// track nodes-behind-NAT connectivity
func backConnect(connector NodeInformer, info warpnet.WarpAddrInfo) {
	if connector.NodeInfo().OwnerId != warpnet.BootstrapOwner {
		return
	}
	err := connector.SimpleConnect(info)
	if err != nil {
		log.Errorf("bootstrap: back-connect failed: %v", err)
		return
	}
	log.Infoln("bootstrap: back-connect success")
}
