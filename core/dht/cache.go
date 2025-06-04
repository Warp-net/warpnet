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
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"strconv"
	"time"
)

type cache struct {
	ec *lru.LRU[string, any]
}

func newLRU() *cache {
	return &cache{
		lru.NewLRU[string, any](256, nil, time.Hour*8),
	}
}

func (c *cache) Add(key, value interface{}) bool {
	innerKey := castKeyToString(key)
	return c.ec.Add(innerKey, value)
}

func (c *cache) Get(key interface{}) (interface{}, bool) {
	innerKey := castKeyToString(key)
	return c.ec.Get(innerKey)
}

func (c *cache) Contains(key interface{}) bool {
	innerKey := castKeyToString(key)
	return c.ec.Contains(innerKey)
}

func (c *cache) Peek(key interface{}) (interface{}, bool) {
	innerKey := castKeyToString(key)
	return c.ec.Peek(innerKey)
}

func (c *cache) Remove(key interface{}) bool {
	innerKey := castKeyToString(key)
	return c.ec.Remove(innerKey)
}

func (c *cache) RemoveOldest() (interface{}, interface{}, bool) {
	k, v, ok := c.ec.RemoveOldest()
	if !ok {
		return nil, nil, false
	}
	return k, v, true
}

func (c *cache) GetOldest() (interface{}, interface{}, bool) {
	k, v, ok := c.ec.GetOldest()
	if !ok {
		return nil, nil, false
	}
	return k, v, true
}

func (c *cache) Keys() []interface{} {
	keys := c.ec.Keys()
	res := make([]interface{}, len(keys))
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

func castKeyToString(key interface{}) string {
	var innerKey string
	switch key.(type) {
	case string:
		innerKey = key.(string)
	case []byte:
		innerKey = string(key.([]byte))
	case []rune:
		innerKey = string(key.([]rune))
	case bool:
		innerKey = strconv.FormatBool(key.(bool))
	case int, int8, int16, int32, int64:
		innerKey = strconv.FormatInt(key.(int64), 10)
	case uint, uint8, uint16, uint32, uint64:
		innerKey = strconv.FormatUint(key.(uint64), 10)
	case float32, float64:
		innerKey = strconv.FormatFloat(key.(float64), 'f', -1, 64)
	default:
		innerKey = fmt.Sprintf("%v", key)
	}
	return innerKey
}
