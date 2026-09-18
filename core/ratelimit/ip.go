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

package ratelimit

import (
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
)

const (
	ipBucketsSize = 4096
	ipBucketsTTL  = time.Minute * 5
)

// IPLimiter is what one remote IP may spend, so that a peer flooding
// discoveries cannot shed the peers everyone else announces.
type IPLimiter struct {
	buckets *Buckets
	limit   Limit
}

func NewIPLimiter(s Settings) *IPLimiter {
	s = s.WithDefaults()
	return &IPLimiter{
		buckets: NewBuckets(ipBucketsSize, ipBucketsTTL),
		limit:   PerTenSeconds(int64(s.DiscoveryBurst), int64(s.DiscoveryPerTenSec)),
	}
}

// Allow charges the bucket of the IP the peer is reachable at. Peers announced
// without an address share a single bucket.
func (l *IPLimiter) Allow(addrs []warpnet.WarpAddress) bool {
	if l == nil {
		return true
	}

	var ip string
	for _, addr := range addrs {
		if parsed := warpnet.MultiAddressIP(addr); parsed != nil {
			ip = parsed.String()
			break
		}
	}
	return l.buckets.Allow(ip, l.limit)
}

func (l *IPLimiter) Close() {
	if l == nil {
		return
	}
	l.buckets.Close()
}
