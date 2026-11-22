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

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"fmt"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

type stubCircle map[string]domain.Trust

func (s stubCircle) Trust(userId string) domain.Trust { return s[userId] }

func TestGetTrustAnswersEveryAskedUser(t *testing.T) {
	circle := stubCircle{"friend": {IsTrusted: true, Score: 1}}
	body, err := json.Marshal(event.GetTrustEvent{UserIds: []string{"friend", "stranger", ""}})
	require.NoError(t, err)

	resp, err := StreamGetTrustHandler(circle)(body, nil)
	require.NoError(t, err)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var got event.GetTrustResponse
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, map[string]domain.Trust{
		"friend":   {IsTrusted: true, Score: 1},
		"stranger": {},
	}, got.Users)
}

func TestGetTrustRefusesOversizeBatch(t *testing.T) {
	ids := make([]string, maxTrustBatch+1)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	body, err := json.Marshal(event.GetTrustEvent{UserIds: ids})
	require.NoError(t, err)

	_, err = StreamGetTrustHandler(stubCircle{})(body, nil)
	require.ErrorIs(t, err, ErrTrustBatchTooLarge)
}
