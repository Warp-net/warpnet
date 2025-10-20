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
	"crypto/ed25519"
	"fmt"
	"io"
	gonet "net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/docker/go-units"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-kad-dht/providers"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	coreconnmgr "github.com/libp2p/go-libp2p/core/connmgr"
	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autonat"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"

	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoreds"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/tcpreuse"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"

	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	tptu "github.com/libp2p/go-libp2p/p2p/net/upgrader"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/multiformats/go-multiaddr"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/net"
	log "github.com/sirupsen/logrus"
)

var ErrAllDialsFailed = swarm.ErrAllDialsFailed

const (
	BootstrapOwner = "bootstrap"
	ModeratorOwner = "moderator"
	WarpnetName    = "warpnet"
	NoiseID        = noise.ID

	Connected = network.Connected
	Limited   = network.Limited

	P_IP4 = multiaddr.P_IP4
	P_IP6 = multiaddr.P_IP6
	P_TCP = multiaddr.P_TCP

	PermanentTTL = peerstore.PermanentAddrTTL

	ErrNodeIsOffline = WarpError("node is offline")
	ErrUserIsOffline = WarpError("user is offline")

	ReachabilityPublic  WarpReachability = network.ReachabilityPublic
	ReachabilityPrivate WarpReachability = network.ReachabilityPrivate
	ReachabilityUnknown WarpReachability = network.ReachabilityUnknown
)

type relayStatus string

const (
	RelayStatusOff     relayStatus = "off"
	RelayStatusWaiting relayStatus = "waiting"
	RelayStatusRunning relayStatus = "running"
)

var (
	privateBlocks = []string{
		"10.0.0.0/8", // VPN
		"172.16.0.0/12",
		"192.168.0.0/16", // private network
		"100.64.0.0/10",  // CG-NAT
		"127.0.0.0/8",    // local
		"169.254.0.0/16", // link-local
	}
)

type WarpError string

func (e WarpError) Error() string {
	return string(e)
}

// interfaces
type (
	WarpRelayCloser interface {
		Close() error
	}
	WarpGossiper interface {
		Close() error
	}
)

type (
	// types
	WarpMDNS        mdns.Service
	WarpPrivateKey  p2pCrypto.PrivKey
	WarpRoutingFunc func(node P2PNode) (WarpPeerRouting, error)

	// aliases
	WarpMessage        = pubsub.Message
	WarpReachability   = network.Reachability
	WarpOption         = libp2p.Option
	WarpLimiterConfig  = rcmgr.PartialLimitConfig
	TCPTransport       = tcp.TcpTransport
	TCPOption          = tcp.Option
	Swarm              = swarm.Swarm
	SwarmOption        = swarm.Option
	PSK                = pnet.PSK
	WarpProtocolID     = protocol.ID
	WarpStream         = network.Stream
	StreamHandler      = network.StreamHandler
	WarpBatching       = datastore.Batching
	WarpProviderStore  = providers.ProviderStore
	WarpAddrInfo       = peer.AddrInfo
	WarpStreamStats    = network.Stats
	WarpPeerRouting    = routing.PeerRouting
	WarpPeerstore      = peerstore.Peerstore
	WarpProtocolSwitch = protocol.Switch
	WarpNetwork        = network.Network
	WarpPeerID         = peer.ID
	WarpDHT            = dht.IpfsDHT
	WarpAddress        = multiaddr.Multiaddr
	WarpConnManager    = coreconnmgr.ConnManager
	WarpEventBus       = event.Bus
	WarpIDService      = identify.IDService
	WarpAutoNAT        = autonat.AutoNAT
	P2PNode            = host.Host
	Discovery          = discovery.Discovery
)

type WarpStreamBody struct {
	WarpStream
	Body []byte
}

type WarpHandlerFunc func(msg []byte, s WarpStream) (any, error)

// structures
type WarpStreamHandler struct {
	Path    WarpProtocolID
	Handler WarpHandlerFunc
}

func (wh *WarpStreamHandler) IsValid() bool {
	if !strings.HasPrefix(string(wh.Path), "/") {
		return false
	}
	if !(strings.Contains(string(wh.Path), "get") ||
		strings.Contains(string(wh.Path), "delete") ||
		strings.Contains(string(wh.Path), "post")) {
		return false
	}
	if !(strings.Contains(string(wh.Path), "private") ||
		strings.Contains(string(wh.Path), "internal") ||
		strings.Contains(string(wh.Path), "public")) {
		return false
	}
	return true
}

func (wh *WarpStreamHandler) String() string {
	return fmt.Sprintf("%s %T", wh.Path, wh.Handler)
}

type WarpPubInfo struct {
	ID    WarpPeerID `json:"peer_id"`
	Addrs []string   `json:"addrs"`
}

type NodeInfo struct {
	OwnerId        string           `json:"owner_id"`
	ID             WarpPeerID       `json:"node_id"`
	Version        *semver.Version  `json:"version"`
	Addresses      []string         `json:"addresses"`
	StartTime      time.Time        `json:"start_time"`
	RelayState     relayStatus      `json:"relay_state"`
	BootstrapPeers []WarpAddrInfo   `json:"bootstrap_peers"`
	Reachability   WarpReachability `json:"reachability"`
	Protocols      []WarpProtocolID `json:"protocols"`
	Hash           string           `json:"hash"`
}

func (ni NodeInfo) IsBootstrap() bool {
	return ni.OwnerId == BootstrapOwner
}
func (ni NodeInfo) IsModerator() bool {
	return ni.OwnerId == ModeratorOwner
}

type NodeStats struct {
	UserId          string          `json:"user_id"`
	NodeID          WarpPeerID      `json:"node_id"`
	Version         *semver.Version `json:"version"`
	PublicAddresses string          `json:"public_addresses"`
	RelayState      relayStatus     `json:"relay_state"`

	StartTime string `json:"start_time"`

	NetworkState string `json:"network_state"`

	DatabaseStats map[string]string `json:"database_stats"`
	MemoryStats   map[string]string `json:"memory_stats"`
	CPUStats      map[string]string `json:"cpu_stats"`

	BytesSent     int64 `json:"bytes_sent"`
	BytesReceived int64 `json:"bytes_received"`

	PeersOnline int `json:"peers_online"`
	PeersStored int `json:"peers_stored"`
}

func NewP2PNode(opts ...libp2p.Option) (P2PNode, error) {
	return libp2p.New(opts...)
}

func NewNoise(id protocol.ID, pk p2pCrypto.PrivKey, mxs []tptu.StreamMuxer) (*noise.Transport, error) {
	return noise.New(id, pk, mxs)
}

func NewTCPTransport(u transport.Upgrader, r network.ResourceManager, s *tcpreuse.ConnMgr, o ...tcp.Option) (*tcp.TcpTransport, error) {
	return tcp.NewTCPTransport(u, r, s, o...)
}

func NewWebsocketTransport(
	u transport.Upgrader,
	r network.ResourceManager,
	s *tcpreuse.ConnMgr,
	o ...websocket.Option,
) (*websocket.WebsocketTransport, error) {
	return websocket.New(u, r, s, o...)
}

func NewConnManager(limiter rcmgr.Limiter) (*connmgr.BasicConnMgr, error) {
	return connmgr.NewConnManager(
		100,
		limiter.GetConnLimits().GetConnTotalLimit(),
		connmgr.WithGracePeriod(time.Hour*12),
	)
}

func NewResourceManager(limiter rcmgr.Limiter) (network.ResourceManager, error) {
	return rcmgr.NewResourceManager(limiter)
}

func NewConfigurableLimiter(input io.Reader) rcmgr.Limiter {
	defaults := rcmgr.DefaultLimits.AutoScale()
	if input == nil {
		return rcmgr.NewFixedLimiter(defaults)
	}
	limiter, err := rcmgr.NewLimiterFromJSON(input, defaults)
	if err != nil {
		log.Error("could not parse limiter config", err)
		return rcmgr.NewFixedLimiter(defaults)
	}
	return limiter
}

func GetMacAddr() string {
	ifas, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var as []string
	for _, ifa := range ifas {
		a := ifa.HardwareAddr
		if a != "" {
			as = append(as, a)
		}
	}
	return strings.Join(as, ",")
}

func GetMemoryStats() map[string]string {
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)

	return map[string]string{
		"heap":    units.HumanSize(float64(memStats.Alloc)),
		"stack":   units.HumanSize(float64(memStats.StackInuse)),
		"last_gc": time.Unix(0, int64(memStats.LastGC)).Format(time.DateTime),
	}
}

func GetCPUStats() map[string]string {
	cpuNum := strconv.Itoa(runtime.NumCPU())

	stats := map[string]string{
		"num": cpuNum,
	}

	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		log.Error("could not get CPU usage percent", err)
		return stats
	}
	if len(percentages) == 0 {
		return stats
	}
	usage := strconv.FormatFloat(percentages[0], 'f', -1, 64)
	stats["usage"] = usage
	return stats
}

func GetNetworkIO() (bytesSent int64, bytesRecv int64) {
	ioCounters, err := net.IOCounters(false) // false = суммарно по всем интерфейсам
	if err != nil {
		log.Error("could not get network io counters", err)
		return 0, 0
	}
	if len(ioCounters) == 0 {
		return 0, 0
	}
	stats := ioCounters[0]
	return int64(stats.BytesSent), int64(stats.BytesRecv)
}

func FromStringToPeerID(s string) WarpPeerID {
	peerID, err := peer.Decode(s)
	if err != nil {
		return ""
	}
	return peerID
}

func FromIDToPubKey(id peer.ID) ed25519.PublicKey {
	pubKey, _ := id.ExtractPublicKey()
	if pubKey == nil {
		return []byte{}
	}
	rawPubKey, _ := pubKey.Raw()
	return rawPubKey
}

func FromBytesToPeerID(b []byte) WarpPeerID {
	peerID, err := peer.IDFromBytes(b)
	if err != nil {
		return ""
	}
	return peerID
}

func NewMultiaddr(s string) (a multiaddr.Multiaddr, err error) {
	return multiaddr.NewMultiaddr(s)
}

func IDFromPublicKey(pk ed25519.PublicKey) (WarpPeerID, error) {
	pub, err := p2pCrypto.UnmarshalEd25519PublicKey(pk)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pub)
}

func UnmarshalEd25519PublicKey(data []byte) (p2pCrypto.PubKey, error) {
	return p2pCrypto.UnmarshalEd25519PublicKey(data)
}

func AddrInfoFromP2pAddr(m multiaddr.Multiaddr) (*WarpAddrInfo, error) {
	return peer.AddrInfoFromP2pAddr(m)
}

func AddrInfoFromString(s string) (*WarpAddrInfo, error) {
	return peer.AddrInfoFromString(s)
}

func NewPeerstore(ctx context.Context, db datastore.Batching) (WarpPeerstore, error) {
	store, err := pstoreds.NewPeerstore(ctx, db, pstoreds.DefaultOpts())
	return WarpPeerstore(store), err
}

func IsPublicMultiAddress(maddr WarpAddress) bool {
	ipStr, err := maddr.ValueForProtocol(P_IP4)
	if err != nil {
		ipStr, err = maddr.ValueForProtocol(P_IP6)
		if err != nil {
			return false
		}
	}
	ip := gonet.ParseIP(ipStr)
	if ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		IsRelayAddress(ip.String()) {
		return false
	}

	for _, block := range privateBlocks {
		_, cidr, _ := gonet.ParseCIDR(block)
		if cidr.Contains(ip) {
			return false
		}
	}
	return true
}

func IsRelayAddress(addr string) bool {
	return strings.Contains(addr, "p2p-circuit")
}

func IsRelayMultiaddress(maddr multiaddr.Multiaddr) bool {
	return strings.Contains(maddr.String(), "p2p-circuit")
}
