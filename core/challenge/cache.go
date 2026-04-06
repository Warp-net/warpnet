package challenge

import (
	"github.com/Warp-net/warpnet/event"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const maxLiveTime = 24 * time.Hour

type nodeEntry struct {
	info          warpnet.WarpAddrInfo
	nextChallenge time.Time
}

type (
	challenge   = []byte
	identinfier = string
)

type challengeEntry struct {
	coordinates []event.ChallengeSample
	challenge   []challenge
}

type challengeCache struct {
	nodesMx *sync.RWMutex
	nodes   map[identinfier]nodeEntry

	coordMx *sync.RWMutex
	coords  map[identinfier]challengeEntry
}

func newChallengeCache() *challengeCache {
	return &challengeCache{
		nodes:   make(map[identinfier]nodeEntry),
		nodesMx: new(sync.RWMutex),
		coordMx: new(sync.RWMutex),
		coords:  make(map[identinfier]challengeEntry),
	}
}

func (chc *challengeCache) IsChallengedAlready(id string) bool {
	chc.nodesMx.RLock()
	defer chc.nodesMx.RUnlock()

	entry, ok := chc.nodes[id]
	if !ok {
		return false
	}

	return time.Now().Before(entry.nextChallenge)
}

func (chc *challengeCache) SetChallenged(id string) {
	chc.nodesMx.Lock()
	defer chc.nodesMx.Unlock()
	entry, ok := chc.nodes[id]
	if !ok {
		entry = nodeEntry{}
	}
	waitPeriod := time.Hour * time.Duration(rand.IntN(8)) //#nosec
	entry.nextChallenge = time.Now().Add(waitPeriod)
	chc.nodes[id] = entry

	for id, e := range chc.nodes {
		if e.nextChallenge.IsZero() || time.Since(e.nextChallenge) > maxLiveTime {
			delete(chc.nodes, id)
		}
	}
}

func (chc *challengeCache) GetFailed(id string) *challengeEntry {
	chc.coordMx.RLock()
	defer chc.coordMx.RUnlock()
	if coord, ok := chc.coords[id]; ok {
		return &coord
	}
	return nil
}

func (chc *challengeCache) SetFailed(id string, ch [][]byte, coord []event.ChallengeSample) {
	chc.coordMx.Lock()
	defer chc.coordMx.Unlock()
	chc.coords[id] = challengeEntry{coordinates: coord, challenge: ch}
}

func (chc *challengeCache) RemoveFailed(id string) {
	chc.coordMx.Lock()
	defer chc.coordMx.Unlock()
	delete(chc.coords, id)
}
