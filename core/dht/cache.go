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

package dht

import (
	"fmt"
	"strconv"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

type cache struct {
	ec *lru.LRU[string, any]
}

func newLRU() *cache {
	return &cache{
		lru.NewLRU[string, any](256, nil, time.Hour*8), //nolint:mnd
	}
}

func (c *cache) Add(key, value any) bool {
	innerKey := castKeyToString(key)
	return c.ec.Add(innerKey, value)
}

func (c *cache) Get(key any) (any, bool) {
	innerKey := castKeyToString(key)
	return c.ec.Get(innerKey)
}

func (c *cache) Contains(key any) bool {
	innerKey := castKeyToString(key)
	return c.ec.Contains(innerKey)
}

func (c *cache) Peek(key any) (any, bool) {
	innerKey := castKeyToString(key)
	return c.ec.Peek(innerKey)
}

func (c *cache) Remove(key any) bool {
	innerKey := castKeyToString(key)
	return c.ec.Remove(innerKey)
}

func (c *cache) RemoveOldest() (any, any, bool) {
	k, v, ok := c.ec.RemoveOldest()
	if !ok {
		return nil, nil, false
	}
	return k, v, true
}

func (c *cache) GetOldest() (any, any, bool) {
	k, v, ok := c.ec.GetOldest()
	if !ok {
		return nil, nil, false
	}
	return k, v, true
}

func (c *cache) Keys() []any {
	keys := c.ec.Keys()
	res := make([]any, len(keys))
	for i, k := range keys {
		res[i] = k
	}
	return res
}

func (c *cache) Len() int {
	return c.ec.Len()
}

func (c *cache) Purge() {
	c.ec.Purge()
}

func (c *cache) Resize(i int) int {
	return c.ec.Resize(i)
}

func castKeyToString(key any) string {
	var innerKey string
	switch typedKey := key.(type) {
	case string:
		innerKey = typedKey
	case []byte:
		innerKey = string(typedKey)
	case []rune:
		innerKey = string(typedKey)
	case bool:
		innerKey = strconv.FormatBool(typedKey)
	case int, int8, int16, int32, int64:
		innerKey = strconv.FormatInt(typedKey.(int64), 10)
	case uint, uint8, uint16, uint32, uint64:
		innerKey = strconv.FormatUint(typedKey.(uint64), 10)
	case float32, float64:
		innerKey = strconv.FormatFloat(typedKey.(float64), 'f', -1, 64)
	default:
		innerKey = fmt.Sprintf("%v", key)
	}
	return innerKey
}
