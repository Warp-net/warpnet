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
	"github.com/Warp-net/warpnet/core/consensus"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type AdminStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type AdminStateCommitter interface {
	CommitState(newState consensus.KVState) (_ *consensus.KVState, err error)
}

type ConsensusResetter interface {
	Reset() error
}

func StreamVerifyHandler(state AdminStateCommitter) middleware.WarpHandler {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		if state == nil {
			return nil, nil
		}
		var newState map[string]string
		err := json.JSON.Unmarshal(buf, &newState)
		if err != nil {
			return nil, err
		}

		log.Infof(
			"node verify request received: %s %s",
			s.Conn().RemotePeer().String(), s.Conn().RemoteMultiaddr().String(),
		)

		updatedState, err := state.CommitState(newState)
		if err != nil {
			return nil, err
		}

		return updatedState, nil
	}
}

func StreamConsensusResetHandler(consRepo ConsensusResetter) middleware.WarpHandler {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		if consRepo == nil {
			return nil, nil
		}

		return event.Accepted, consRepo.Reset()
	}
}
