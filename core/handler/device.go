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
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/keepalive"
	log "github.com/sirupsen/logrus"
)

type NodeAddresser interface {
	Addrs() []warpnet.WarpAddress
}

type AliasStorer interface {
	SetAlias(a domain.Alias) error
	GetAliases() (aliases []domain.Alias, err error)
	DeleteAlias(nodeId string) error
}

type PairAuthStorer interface {
	SessionToken() string
}

func StreamNodesPairingHandler(
	authRepo PairAuthStorer,
	aliasesRepo AliasStorer,
	n NodeAddresser,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var clientInfo domain.AuthNodeInfo
		if err := json.Unmarshal(buf, &clientInfo); err != nil {
			log.Errorf("pair: unmarshaling from stream: %s %v", buf, err)
			return nil, err
		}
		if clientInfo.Token == "" {
			return nil, warpnet.WarpError("empty token")
		}

		if authRepo.SessionToken() != clientInfo.Token {
			log.Errorf("pair: token does not match server identity")
			return nil, warpnet.WarpError("token mismatch")
		}

		if err := aliasesRepo.SetAlias(domain.Alias{
			NodeId: s.Conn().RemotePeer().String(),
			Token:  clientInfo.Token,
		}); err != nil {
			return nil, err
		}

		if err := keepalive.Acquire(); err != nil {
			log.Errorf("pair: keepalive acquire: %v", err)
		}

		println()
		fmt.Printf(
			"\033[1mPAIRED %s\033[0m\n",
			s.Conn().RemotePeer().String(),
		)
		println()

		nodeAddrs := n.Addrs()
		addrs := make([]string, 0, len(nodeAddrs))
		for _, addr := range nodeAddrs {
			addrs = append(addrs, addr.String())
		}

		return addrs, nil
	}
}

// StreamGetDevicesHandler answers with the devices paired to this node.
func StreamGetDevicesHandler(aliasesRepo AliasStorer) warpnet.WarpHandlerFunc {
	return func(_ []byte, _ warpnet.WarpStream) (any, error) {
		aliases, err := aliasesRepo.GetAliases()
		if err != nil {
			log.Errorf("devices: listing: %v", err)
			return nil, err
		}
		return event.GetDevicesResponse{Devices: toDeviceInfos(aliases)}, nil
	}
}

// toDeviceInfos strips each alias down to its wire-safe fields — a paired
// device reaching this route must never learn another device's token.
func toDeviceInfos(aliases []domain.Alias) []event.DeviceInfo {
	infos := make([]event.DeviceInfo, 0, len(aliases))
	for _, a := range aliases {
		infos = append(infos, event.DeviceInfo{
			NodeId:     a.NodeId,
			Platform:   a.Platform,
			CreatedAt:  a.CreatedAt,
			LastActive: a.LastActive,
		})
	}
	return infos
}

// StreamDeleteDeviceHandler unpairs a device. The device keeps its copy of
// the pairing payload, but its peer id no longer authorizes anything on
// this node.
func StreamDeleteDeviceHandler(aliasesRepo AliasStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteDeviceEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			log.Errorf("devices: unmarshaling from stream: %s %v", buf, err)
			return nil, err
		}
		if ev.NodeId == "" {
			return nil, warpnet.WarpError("devices: empty node id")
		}

		// A paired device may unpair itself and nothing else. The owner's
		// own UI reaches this over a loopback stream, which is no alias.
		if p, ok := s.(warpnet.PairedAliasStream); ok &&
			p.IsPairedAlias() &&
			ev.NodeId != s.Conn().RemotePeer().String() {
			return nil, warpnet.WarpError("devices: a device may only unpair itself")
		}

		if err := aliasesRepo.DeleteAlias(string(ev.NodeId)); err != nil {
			log.Errorf("devices: deleting %s: %v", ev.NodeId, err)
			return nil, err
		}

		aliases, err := aliasesRepo.GetAliases()
		if err != nil {
			log.Errorf("devices: listing: %v", err)
			return nil, err
		}
		if len(aliases) == 0 {
			if err := keepalive.Release(); err != nil {
				log.Errorf("devices: keepalive release: %v", err)
			}
		}
		return event.GetDevicesResponse{Devices: toDeviceInfos(aliases)}, nil
	}
}
