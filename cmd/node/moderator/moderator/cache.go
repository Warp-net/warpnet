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
package moderator

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
)

const maxLiveTime = 8 * time.Hour

type CacheEntry struct {
	Result         event.ModerationResultEvent
	expirationTime time.Time
}

type moderationCache struct {
	mx    *sync.RWMutex
	peers map[warpnet.WarpPeerID]CacheEntry
}

func newModerationCache() *moderationCache {
	return &moderationCache{
		peers: make(map[warpnet.WarpPeerID]CacheEntry),
		mx:    new(sync.RWMutex),
	}
}

func (dc *moderationCache) IsModeratedAlready(id warpnet.WarpPeerID) bool {
	dc.mx.RLock()
	defer dc.mx.RUnlock()

	entry, ok := dc.peers[id]
	if !ok {
		return false
	}

	isExpired := time.Now().After(entry.expirationTime)
	if isExpired {
		delete(dc.peers, id)
		return false
	}
	return true
}

func (dc *moderationCache) SetAsModerated(peerId warpnet.WarpPeerID, entry CacheEntry) {
	dc.mx.Lock()
	defer dc.mx.Unlock()

	waitPeriod := time.Minute * time.Duration(rand.IntN(8))
	entry.expirationTime = time.Now().Add(waitPeriod)
	dc.peers[peerId] = entry

	for id, e := range dc.peers {
		if !e.expirationTime.IsZero() && time.Since(e.expirationTime) < maxLiveTime {
			continue
		}

		delete(dc.peers, id)
	}
	return
}
