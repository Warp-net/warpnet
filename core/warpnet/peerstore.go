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

package warpnet

import (
	"maps"
	"slices"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const protocolsCacheSize = 256

type certifiedPeerstore interface {
	peerstore.Peerstore
	peerstore.CertifiedAddrBook
}

type cachedPeerstore struct {
	certifiedPeerstore

	mx        sync.Mutex
	protocols *lru.Cache[peer.ID, map[protocol.ID]struct{}]
}

func newCachedPeerstore(store certifiedPeerstore) (*cachedPeerstore, error) {
	protocols, err := lru.New[peer.ID, map[protocol.ID]struct{}](protocolsCacheSize)
	if err != nil {
		return nil, err
	}
	return &cachedPeerstore{certifiedPeerstore: store, protocols: protocols}, nil
}

func (ps *cachedPeerstore) GetProtocols(p peer.ID) ([]protocol.ID, error) {
	protocols, err := ps.loadProtocols(p)
	if err != nil {
		return nil, err
	}
	return slices.Collect(maps.Keys(protocols)), nil
}

func (ps *cachedPeerstore) SupportsProtocols(p peer.ID, protos ...protocol.ID) ([]protocol.ID, error) {
	protocols, err := ps.loadProtocols(p)
	if err != nil {
		return nil, err
	}
	supported := make([]protocol.ID, 0, len(protos))
	for _, proto := range protos {
		if _, ok := protocols[proto]; ok {
			supported = append(supported, proto)
		}
	}
	return supported, nil
}

func (ps *cachedPeerstore) FirstSupportedProtocol(p peer.ID, protos ...protocol.ID) (protocol.ID, error) {
	protocols, err := ps.loadProtocols(p)
	if err != nil {
		return "", err
	}
	for _, proto := range protos {
		if _, ok := protocols[proto]; ok {
			return proto, nil
		}
	}
	return "", nil
}

func (ps *cachedPeerstore) SetProtocols(p peer.ID, protos ...protocol.ID) error {
	ps.mx.Lock()
	defer ps.mx.Unlock()
	ps.protocols.Remove(p)
	return ps.certifiedPeerstore.SetProtocols(p, protos...)
}

func (ps *cachedPeerstore) AddProtocols(p peer.ID, protos ...protocol.ID) error {
	ps.mx.Lock()
	defer ps.mx.Unlock()
	ps.protocols.Remove(p)
	return ps.certifiedPeerstore.AddProtocols(p, protos...)
}

func (ps *cachedPeerstore) RemoveProtocols(p peer.ID, protos ...protocol.ID) error {
	ps.mx.Lock()
	defer ps.mx.Unlock()
	ps.protocols.Remove(p)
	return ps.certifiedPeerstore.RemoveProtocols(p, protos...)
}

func (ps *cachedPeerstore) RemovePeer(p peer.ID) {
	ps.mx.Lock()
	defer ps.mx.Unlock()
	ps.protocols.Remove(p)
	ps.certifiedPeerstore.RemovePeer(p)
}

func (ps *cachedPeerstore) loadProtocols(p peer.ID) (map[protocol.ID]struct{}, error) {
	if protocols, ok := ps.protocols.Get(p); ok {
		return protocols, nil
	}
	ps.mx.Lock()
	defer ps.mx.Unlock()
	list, err := ps.certifiedPeerstore.GetProtocols(p)
	if err != nil {
		return nil, err
	}
	protocols := make(map[protocol.ID]struct{}, len(list))
	for _, proto := range list {
		protocols[proto] = struct{}{}
	}
	ps.protocols.Add(p, protocols)
	return protocols, nil
}
