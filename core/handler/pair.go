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
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type NodeAddresser interface {
	PublicAddrs() []warpnet.WarpAddress
}

type DeviceStorer interface {
	SetDevice(ownerNodeId string, device domain.Device) error
}

// PairAuthStorer exposes the node's current session token. The handler reads it
// per request (not a value captured at registration) so a re-authenticated node
// — e.g. a business node that seals its DB on dashboard close and reopens on the
// next login, minting a fresh token — still matches a freshly scanned QR.
type PairAuthStorer interface {
	SessionToken() string
}

// StreamNodesPairingHandler authorizes a device whose presented token matches
// the node's current session token, read from authRepo per request (see
// PairAuthStorer) so a re-authenticated node still matches a freshly scanned QR
// instead of failing with "token mismatch".
func StreamNodesPairingHandler(authRepo PairAuthStorer, deviceRepo DeviceStorer, n NodeAddresser) warpnet.WarpHandlerFunc {
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

		if err := deviceRepo.SetDevice(s.Conn().LocalPeer().String(), domain.Device{
			NodeId: s.Conn().RemotePeer(),
			Token:  clientInfo.Token,
		}); err != nil {
			return nil, err
		}

		println()
		fmt.Printf(
			"\033[1mPAIRED %s\033[0m\n",
			s.Conn().RemotePeer().String(),
		)
		println()

		addrs := make([]string, 0, len(n.PublicAddrs()))
		for _, addr := range n.PublicAddrs() {
			addrs = append(addrs, addr.String())
		}

		return addrs, nil
	}
}
