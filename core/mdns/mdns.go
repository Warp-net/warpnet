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

package mdns

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	log "github.com/sirupsen/logrus"
)

/*
  Multicast DNS (mDNS) is a protocol that allows devices to discover each other on a local network
  without the need for centralized servers. It works by sending multicast DNS queries,
  enabling nodes to find other nodes by hostname.

  ### **mDNS is used in:**
  - Apple Bonjour (automatic device discovery in networks)
  - Google Chromecast, AirPlay
  - IoT devices and P2P networks
  - Decentralized systems where no central server is available for node announcements

  The **go-libp2p-mdns** library is an implementation of mDNS for libp2p, designed for
  automatic peer discovery in local networks without requiring a centralized server.

  ### **Key Features of go-libp2p-mdns:**
  - **Local Peer Discovery**
    - Enables automatic detection of network participants without manual configuration.
  - **Works via UDP Multicast Queries**
    - Allows DNS queries to be transmitted without using a traditional DNS server.
  - **Flexibility**
    - Suitable for temporary (Ad-Hoc) networks and local P2P systems.
  - **Zero-Configuration**
    - Simplifies the deployment of P2P applications without requiring manual node address setup.
  - **Optimized for Small Networks**
    - Ideal for local applications but inefficient for large-scale global P2P networks.
*/

type NodeConnector interface {
	Connect(peer.AddrInfo) error
	Node() warpnet.P2PNode
}

type MulticastDNS struct {
	mdns      warpnet.WarpMDNS
	service   *mdnsDiscoveryService
	isRunning *atomic.Bool
}

type mdnsDiscoveryService struct {
	ctx              context.Context
	discoveryHandler discovery.DiscoveryHandler
	node             NodeConnector
	mx               *sync.Mutex
}

func (m *mdnsDiscoveryService) HandlePeerFound(p peer.AddrInfo) {
	if m == nil {
		return
	}
	if m.node == nil {
		panic("mdns: node is nil")
	}

	m.mx.Lock()
	defer m.mx.Unlock()

	if m.discoveryHandler != nil {
		m.discoveryHandler(p)
		return
	}
	m.defaultDiscoveryHandler(p)
}

func (m *mdnsDiscoveryService) defaultDiscoveryHandler(peerInfo warpnet.WarpAddrInfo) {
	err := m.node.Connect(peerInfo)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("mdns: discovery: backoffed: %s %s", peerInfo.Addrs, peerInfo.ID)
		return
	}
	if err != nil {
		log.Errorf("mdns: discovery: failed to connect to peer %s: %v", peerInfo.String(), err)
		return
	}
	log.Debugf("mdns: discovery: connected to peer: %s %s", peerInfo.Addrs, peerInfo.ID)
}

func NewMulticastDNS(ctx context.Context, discoveryHandler discovery.DiscoveryHandler) *MulticastDNS {
	service := &mdnsDiscoveryService{
		ctx:              ctx,
		discoveryHandler: discoveryHandler,
		node:             nil,
		mx:               new(sync.Mutex),
	}

	return &MulticastDNS{nil, service, new(atomic.Bool)}
}

func (m *MulticastDNS) Start(n NodeConnector) {
	if m == nil {
		return
	}
	if m.isRunning.Load() {
		return
	}
	m.isRunning.Store(true)

	m.service.mx.Lock()
	m.service.node = n
	m.service.mx.Unlock()

	m.mdns = mdns.NewMdnsService(n.Node(), warpnet.WarpnetName, m.service)

	go func() {
		if err := m.mdns.Start(); err != nil {
			log.Errorf("mdns: failed to start: %v", err)
			return
		}
		log.Infoln("mdns: service started")
	}()
}

func (m *MulticastDNS) Close() {
	if m == nil || m.mdns == nil {
		return
	}
	if !m.isRunning.Load() {
		return
	}
	if err := m.mdns.Close(); err != nil {
		log.Errorf("mdns: failed to close: %v", err)
	}
	m.isRunning.Store(false)
}
