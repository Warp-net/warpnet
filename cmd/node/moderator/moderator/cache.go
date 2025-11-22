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
	"time"

	"github.com/Warp-net/warpnet/domain"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

type CacheKey struct {
	Type   domain.ModerationObjectType
	PeerId string
	Cursor *string
}

type tweetModerationCache struct {
	cache *lru.LRU[CacheKey, struct{}]
}

func newTweetModerationCache() *tweetModerationCache {
	return &tweetModerationCache{lru.NewLRU[CacheKey, struct{}](256, nil, time.Hour*24)} //nolint:mnd
}

func (tmc *tweetModerationCache) IsModeratedAlready(key CacheKey) bool {
	return tmc.cache.Contains(key)
}

func (tmc *tweetModerationCache) SetAsModerated(key CacheKey) {
	tmc.cache.Add(key, struct{}{})
}
