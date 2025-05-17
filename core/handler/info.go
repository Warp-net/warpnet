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
}

func StreamGetInfoHandler(
	i NodeInformer,
	handler discovery.DiscoveryHandler,
) warpnet.WarpStreamHandler {
	return func(s warpnet.WarpStream) {
		defer func() { s.Close() }() //#nosec

		remoteAddr := s.Conn().RemoteMultiaddr()

		log.Debugf("node info request received: %s %s", s.Conn().RemotePeer().String(), remoteAddr)

		if handler != nil {
			handler(warpnet.PeerAddrInfo{
				ID:    s.Conn().RemotePeer(),
				Addrs: []warpnet.WarpAddress{remoteAddr},
			})
		}

		if err := json.JSON.NewEncoder(s).Encode(i.NodeInfo()); err != nil {
			log.Errorf("fail encoding generic response: %v", err)
		}
		return
	}
}
