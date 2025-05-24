package discovery

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"math/rand/v2"
	"sync"
	"time"
)

const maxLiveTime = 24 * time.Hour

type cacheEntry struct {
	info          warpnet.WarpAddrInfo
	nextChallenge time.Time
}

type discoveryCache struct {
	mx    *sync.RWMutex
	nodes map[warpnet.WarpPeerID]cacheEntry
}

func newDiscoveryCache() *discoveryCache {
	return &discoveryCache{
		nodes: make(map[warpnet.WarpPeerID]cacheEntry),
		mx:    new(sync.RWMutex),
	}
}

func (dc *discoveryCache) IsChallengedAlready(id warpnet.WarpPeerID) bool {
	dc.mx.RLock()
	defer dc.mx.RUnlock()

	entry, ok := dc.nodes[id]
	if !ok {
		return false
	}

	return time.Now().Before(entry.nextChallenge)
}

func (dc *discoveryCache) SetAsChallenged(peerId warpnet.WarpPeerID) {
	dc.mx.Lock()
	defer dc.mx.Unlock()
	entry, ok := dc.nodes[peerId]
	if !ok {
		entry = cacheEntry{}
	}
	waitPeriod := time.Hour * time.Duration(rand.IntN(8))
	entry.nextChallenge = time.Now().Add(waitPeriod)
	dc.nodes[peerId] = entry

	for id, e := range dc.nodes {
		if e.nextChallenge.IsZero() || time.Since(e.nextChallenge) > maxLiveTime {
			delete(dc.nodes, id)
		}
	}
	return
}
