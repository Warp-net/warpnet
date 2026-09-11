package node

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"sync"
	"time"
)

const (
	reachabilityTag = "reachability"
	// ratingTag is kept apart from reachabilityTag so the two compose the
	// way libp2p intends, instead of overwriting each other.
	ratingTag      = "rating"
	flappingPeriod = 30 * time.Second
	cacheSize      = 128
)

type nodeReachabilityManager struct {
	mx      sync.Mutex
	flapLRU *expirable.LRU[string, struct{}]
	manager warpnet.WarpConnManager
}

func newNodeReachabilityManager(cm warpnet.WarpConnManager) *nodeReachabilityManager {
	lru := expirable.NewLRU[string, struct{}](cacheSize, nil, flappingPeriod)
	return &nodeReachabilityManager{
		flapLRU: lru,
		manager: cm,
	}
}

// SetRatingPriority is what a peer is worth to the connection manager
// when it has to choose whom to keep. Unlike reachability it ignores the
// flap window: a standing changes slowly enough already.
func (m *nodeReachabilityManager) SetRatingPriority(pid warpnet.WarpPeerID, tag int) {
	if m == nil || m.manager == nil {
		return
	}
	m.manager.UpsertTag(pid, ratingTag, func(int) int { return tag })
}

func (m *nodeReachabilityManager) SetPriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability) {
	m.mx.Lock()
	if m.flapLRU.Contains(pid.String()) {
		m.mx.Unlock()
		return
	}

	switch r {
	case warpnet.ReachabilityPublic:
		m.manager.UpsertTag(pid, reachabilityTag, func(old int) int {
			return 90
		})
	case warpnet.ReachabilityUnknown:
		m.manager.UpsertTag(pid, reachabilityTag, func(old int) int {
			return 60
		})
	case warpnet.ReachabilityPrivate:
		m.manager.UpsertTag(pid, reachabilityTag, func(old int) int {
			return 30
		})
	}
	m.flapLRU.Add(pid.String(), struct{}{})
	m.mx.Unlock()
}

func (m *nodeReachabilityManager) SetMinPriority(pid warpnet.WarpPeerID) {
	m.mx.Lock()
	if m.flapLRU.Contains(pid.String()) {
		m.mx.Unlock()
		return
	}
	m.manager.UpsertTag(pid, reachabilityTag, func(i int) int {
		return 1
	})
	m.flapLRU.Add(pid.String(), struct{}{})
	m.mx.Unlock()
}

func (m *nodeReachabilityManager) SetMaxPriority(pid warpnet.WarpPeerID) {
	m.mx.Lock()
	if m.flapLRU.Contains(pid.String()) {
		m.mx.Unlock()
		return
	}
	m.manager.UpsertTag(pid, reachabilityTag, func(i int) int {
		return 100
	})
	m.flapLRU.Add(pid.String(), struct{}{})
	m.mx.Unlock()
}
