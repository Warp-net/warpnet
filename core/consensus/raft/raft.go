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

package raft

import (
	"context"
	"github.com/Warp-net/warpnet/core/warpnet"
	raft "github.com/filinvadim/libp2p-raft-go"
)

/*
		Raft is a consensus algorithm designed for managing replicated logs in distributed systems.
		It was developed as a more understandable alternative to Paxos and is used to ensure data consistency across nodes.

	  Raft solves three key tasks:
	  1. **Leader Election**: One node is elected as the leader, responsible for managing log entries.
	  2. **Log Replication**: The leader accepts commands and distributes them to other nodes for synchronization.
	  3. **Safety and Fault Tolerance**: Ensures that data remains consistent even in the event of failures.

	  Raft provides **strong consistency**, making it suitable for distributed systems that require predictability
	  and protection against network partitioning.

	  The **go-libp2p-consensus** library is a module for libp2p that enables the integration of consensus mechanisms
	  (including Raft) into peer-to-peer (P2P) networks. It provides an abstract interface that can be implemented
	  for various consensus algorithms, including Raft, PoW, PoS, and BFT-based systems.

	  ### **Key Features of go-libp2p-consensus:**
	  - **Consensus Algorithm Abstraction**
	    - Supports Raft and other algorithms (e.g., PoS).
	  - **Integration with libp2p**
	    - Designed for decentralized systems without a central coordinator.
	  - **Flexibility**
	    - Developers can implement custom consensus logic by extending the library's interfaces.
	  - **Optimized for P2P Environments**
	    - Unlike the traditional Raft, it is adapted for dynamically changing networks.
*/

// TODO unused for now. Wait for Raft Tree implementation
type raftNode struct {
	*raft.ConsensusService
}

func NewRaft(
	ctx context.Context,
	store raft.StableStorer,
	isInitiator bool,
	node warpnet.P2PNode,
	addrInfos []warpnet.WarpAddrInfo,
	validators ...raft.ConsensusValidatorFunc,
) (_ *raftNode, err error) {
	config := &raft.Config{
		FSM:                raft.NewFSM(&raft.DefaultCodec{}, validators...),
		Codec:              &raft.DefaultCodec{},
		StableStore:        store,
		LogStore:           nil, // in-mem
		BootstrapNodes:     addrInfos,
		IsClusterInitiator: isInitiator,
		Logger:             raft.DefaultConsensusLogger(),
		Validators:         validators,
		LeaderProtocolID:   "/public/post/admin/verifynode/0.0.0",
		TransportFunc:      raft.NewConsensusTransport,
	}

	svc, err := raft.NewLibP2pRaft(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := svc.Start(node); err != nil {
		return nil, err
	}
	return &raftNode{svc}, nil
}
