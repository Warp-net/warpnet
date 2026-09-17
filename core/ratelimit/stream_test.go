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

//nolint:all
package ratelimit

import (
	"testing"

	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
)

func TestStreamLimitsCarryTheOwnersBudgets(t *testing.T) {
	limits := NewStreamLimits(Settings{
		StreamReadBurst: 7, StreamReadPerMinute: 70,
		StreamWriteBurst: 3, StreamWritePerMinute: 30,
	})

	assert.Equal(t, PerMinute(7, 70), limits.Read())
	assert.Equal(t, PerMinute(3, 30), limits.Write())

	read, ok := limits.Route(event.PUBLIC_POST_VIEW)
	assert.True(t, ok)
	assert.Equal(t, limits.Read(), read, "a route that is a read in all but name follows the read budget")

	media, ok := limits.Route(event.PUBLIC_GET_IMAGE)
	assert.True(t, ok)
	assert.Equal(t, PerMinute(150, 600), media, "a route with a budget of its own keeps it")

	_, ok = limits.Route(event.PUBLIC_GET_TWEETS)
	assert.False(t, ok, "a route without its own budget falls back to read or write")
}

func TestStreamLimitsFallBackToTheDefaults(t *testing.T) {
	limits := NewStreamLimits(Settings{})

	assert.Equal(t, PerMinute(int64(Defaults.StreamReadBurst), int64(Defaults.StreamReadPerMinute)), limits.Read())
	assert.Equal(t, PerMinute(int64(Defaults.StreamWriteBurst), int64(Defaults.StreamWritePerMinute)), limits.Write())
	assert.Equal(t, PerMinute(600, 6000), limits.Gateway())
}
