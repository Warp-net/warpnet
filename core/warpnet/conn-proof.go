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

package warpnet

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// connProofTimeout bounds one connection proof run from a handler.
const connProofTimeout = 10 * time.Second

const ErrConnNotProven = WarpError("connection did not prove the peer's identity")

// ConnProver checks that the far end of c holds a key beyond its peer id.
type ConnProver func(ctx context.Context, c WarpConn) error

// connProof guards a peer whose libp2p key is not secret (the ActivityPub
// gateway): each connection to it must pass the prover before it is trusted.
var connProof = struct {
	sync.Mutex
	peer   WarpPeerID
	prove  ConnProver
	proven map[string]struct{}
}{proven: map[string]struct{}{}}

// RequireConnProof makes every connection to peer prove itself with prove.
func RequireConnProof(peer WarpPeerID, prove ConnProver) {
	connProof.Lock()
	defer connProof.Unlock()
	connProof.peer, connProof.prove = peer, prove
	connProof.proven = map[string]struct{}{}
}

// ProveConn returns nil when c needs no proof or has passed it. A connection
// that fails is closed, so a retry dials another one.
func ProveConn(ctx context.Context, c WarpConn) error {
	connProof.Lock()
	peer, prove := connProof.peer, connProof.prove
	connProof.Unlock()
	if prove == nil {
		return nil
	}
	if c == nil {
		return ErrConnNotProven
	}
	if c.RemotePeer() != peer {
		return nil
	}
	connProof.Lock()
	_, ok := connProof.proven[c.ID()]
	connProof.Unlock()
	if ok {
		return nil
	}
	if err := prove(ctx, c); err != nil {
		_ = c.Close()
		return ErrConnNotProven
	}
	connProof.Lock()
	connProof.proven[c.ID()] = struct{}{}
	connProof.Unlock()
	return nil
}

// ForgetConn drops a closed connection's proof.
func ForgetConn(c WarpConn) {
	connProof.Lock()
	delete(connProof.proven, c.ID())
	connProof.Unlock()
}

// ForgetConnNotifiee drops each connection's proof once it closes.
func ForgetConnNotifiee() network.Notifiee {
	return &network.NotifyBundle{
		DisconnectedF: func(_ network.Network, c network.Conn) { ForgetConn(c) },
	}
}
