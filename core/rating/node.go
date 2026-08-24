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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package rating

import (
	"context"
	"crypto/ed25519"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
)

// NewNodeStore builds the store for a running node: dimensions from its
// role, acquaintance from its live connections, records replicated
// through the node's one CRDT replica. Replication is the point of
// putting rating on a CRDT at all — a node with no disk gets its view
// back from the DAG after a restart and cannot recover it any other way.
func NewNodeStore(
	ctx context.Context,
	replica Replica,
	node warpnet.P2PNode,
	privKey ed25519.PrivateKey,
	nodeType string,
) (*Store, error) {
	if node == nil || replica == nil {
		return nil, ErrNoNode
	}
	store, err := NewStore(Config{
		Ctx:        ctx,
		Self:       node.ID(),
		PrivKey:    privKey,
		Dimensions: DimensionsFor(nodeType),
		Acquainted: connectionAge{host: node},
	}, replica)
	if err != nil {
		return nil, err
	}
	log.Infof("rating: store started for a %s node", nodeType)
	return store, nil
}

// connectionAge answers how long we have been connected to a peer,
// which is what gates whether a remote observer's records count. It
// reads libp2p's own connection stats rather than keeping a second
// bookkeeping of the same thing.
type connectionAge struct {
	host warpnet.P2PNode
}

func (c connectionAge) ConnectedSince(id warpnet.WarpPeerID) (time.Time, bool) {
	if c.host == nil || c.host.Network() == nil {
		return time.Time{}, false
	}
	conns := c.host.Network().ConnsToPeer(id)
	if len(conns) == 0 {
		return time.Time{}, false
	}
	// The oldest live connection: a peer that reconnects should not
	// reset its acquaintance, but one that has genuinely just arrived
	// should not inherit any either.
	oldest := time.Time{}
	for _, conn := range conns {
		opened := conn.Stat().Opened
		if oldest.IsZero() || opened.Before(oldest) {
			oldest = opened
		}
	}
	return oldest, !oldest.IsZero()
}
