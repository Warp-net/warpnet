package socks5

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/huandu/skiplist"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/time/rate"
	"hash/fnv"
	"io"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"time"
)

type socksBalancer struct {
	streamer Streamer
	client   *http.Client

	mx               sync.RWMutex
	latencyList      *skiplist.SkipList
	peersWithLatency map[string]int64
	ruIPCache        *simplelru.LRU[string, struct{}]
	rateLimiter      *rate.Limiter

	stopChan chan struct{}
}

func newBalancer(ctx context.Context, streamer Streamer) *socksBalancer {
	lru, _ := simplelru.NewLRU[string, struct{}](3000, nil)

	b := &socksBalancer{
		streamer: streamer,
		client: &http.Client{
			Timeout: time.Second,
		},
		latencyList:      skiplist.New(skiplist.Int64),
		stopChan:         make(chan struct{}),
		peersWithLatency: make(map[string]int64),
		mx:               sync.RWMutex{},
		rateLimiter:      rate.NewLimiter(rate.Every(300*time.Millisecond), 1),
		ruIPCache:        lru,
	}
	go b.trackExitNodes(ctx)
	return b
}

const topK = 3

func (b *socksBalancer) GetFastestPeer() warpnet.WarpPeerID {
	b.mx.RLock()
	defer b.mx.RUnlock()

	if b.latencyList.Len() == 0 {
		peers := b.streamer.Peerstore().PeersWithAddrs()
		if len(peers) != 0 {
			return peers[rand.Intn(len(peers))] //nolint:gosec
		}
		return ""
	}

	candidates := make([]warpnet.WarpPeerID, 0, topK)

	count := 0
	for element := b.latencyList.Front(); element != nil && count < topK; element = element.Next() {
		peer, ok := element.Value.(warpnet.WarpPeerID)
		if !ok {
			continue
		}
		candidates = append(candidates, peer)
		count++
	}

	if len(candidates) == 0 {
		return ""
	}

	return candidates[rand.Intn(len(candidates))] //nolint:gosec
}

const latencyRefreshDuration = time.Second * 30

func (b *socksBalancer) trackExitNodes(ctx context.Context) {
	ticker := time.NewTicker(latencyRefreshDuration)
	defer ticker.Stop()
	b.detectSuitablePeers(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopChan:
			return
		case <-ticker.C:
			b.detectSuitablePeers(ctx)
		}
	}
}

func (b *socksBalancer) detectSuitablePeers(ctx context.Context) {
	p2pStore := b.streamer.Peerstore()
	p2pNet := b.streamer.Network()

	peers := p2pStore.PeersWithAddrs()
	if len(peers) == 0 {
		return
	}
	for _, peer := range peers {
		if b.streamer.NodeInfo().ID == peer {
			continue
		}
		if p2pNet.Connectedness(peer) == network.NotConnected {
			if err := b.streamer.SimpleConnect(p2pStore.PeerInfo(peer)); err != nil {
				continue
			}
		}
		protocols, _ := p2pStore.GetProtocols(peer)
		if len(protocols) == 0 || !slices.Contains(protocols, DefaultStreamProtocol) {
			continue
		}

		if b.ruIPCache.Contains(peer.String()) {
			continue
		}
		addrInfo := p2pStore.PeerInfo(peer)
		if b.isRestrictedPeer(ctx, addrInfo) { // skip Restricted exit nodes
			b.ruIPCache.Add(peer.String(), struct{}{})
			continue
		}

		latency := p2pStore.LatencyEWMA(peer)
		key := makeKey(peer.String(), latency)

		b.mx.Lock()

		if prevKey, ok := b.peersWithLatency[peer.String()]; ok {
			b.latencyList.Remove(prevKey)
			delete(b.peersWithLatency, peer.String())
		}
		b.latencyList.Set(key, peer)
		b.peersWithLatency[peer.String()] = key

		b.mx.Unlock()
	}
}

func (b *socksBalancer) IsRestrictedPeer(peer warpnet.WarpPeerID) bool {
	return b.ruIPCache.Contains(peer.String())
}

func (b *socksBalancer) Close() {
	defer func() { recover() }() //nolint:errcheck
	close(b.stopChan)
}

func (b *socksBalancer) isRestrictedPeer(ctx context.Context, info warpnet.WarpAddrInfo) bool {
	if len(info.Addrs) == 0 {
		return false
	}
	for _, addr := range info.Addrs {
		if ip, err := addr.ValueForProtocol(multiaddr.P_IP4); err == nil {
			if b.isRestrictedIP(ctx, ip) {
				return true
			}
		}
		if ip, err := addr.ValueForProtocol(multiaddr.P_IP6); err == nil {
			if b.isRestrictedIP(ctx, ip) {
				return true
			}
		}
	}
	return false
}

const (
	geoAPI = "http://ip-api.com/json/" //nolint:nosec

	ruCode = "RU"
	ruName = "Russia"

	chCode = "CH"
	chName = "China"

	byCode = "BY"
	byName = "Belarus"

	irCode = "IR"
	irName = "Iran"
)

var restrictedCountries = map[string]bool{
	ruCode: true,
	ruName: true,
	chCode: true,
	chName: true,
	byCode: true,
	byName: true,
	irCode: true,
	irName: true,
}

type GeoResponse struct {
	Status      string `json:"status"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
}

func (b *socksBalancer) isRestrictedIP(ctx context.Context, ip string) bool {
	if err := b.rateLimiter.Wait(ctx); err != nil {
		return false
	}
	resp, err := b.client.Get(geoAPI + ip) //nolint:noctx
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	fmt.Println(string(body), "BODY")

	var response GeoResponse
	if err := jsoniter.Unmarshal(body, &response); err != nil {
		return false
	}
	if response.Status != "success" {
		return false
	}
	return restrictedCountries[response.Country] || restrictedCountries[response.CountryCode]
}

func makeKey(peerID string, latency time.Duration) int64 {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(peerID))

	hash := hasher.Sum32()
	return (latency.Milliseconds() << 32) | int64(hash)
}
