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

package database

import (
"context"
"testing"
"time"

"github.com/Warp-net/warpnet/core/crdt"
"github.com/Warp-net/warpnet/database/local"
"github.com/stretchr/testify/require"
)

// mockBroadcaster implements crdt.Broadcaster for testing
type mockBroadcaster struct {
data chan []byte
}

func newMockBroadcaster() *mockBroadcaster {
return &mockBroadcaster{
data: make(chan []byte, 100),
}
}

func (m *mockBroadcaster) Broadcast(ctx context.Context, data []byte) error {
select {
case m.data <- data:
return nil
case <-ctx.Done():
return ctx.Err()
default:
return nil
}
}

func (m *mockBroadcaster) Next(ctx context.Context) ([]byte, error) {
select {
case data := <-m.data:
return data, nil
case <-ctx.Done():
return nil, ctx.Err()
case <-time.After(100 * time.Millisecond):
return nil, context.DeadlineExceeded
}
}

func TestCRDTLikeRepo(t *testing.T) {
if testing.Short() {
t.Skip("skipping test in short mode")
}

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// Setup local database
db, err := local.New("", local.DefaultOptions().WithInMemory(true))
require.NoError(t, err)
defer db.Close()

// Create mock broadcaster
broadcaster := newMockBroadcaster()

// Create CRDT stats store
statsStore, err := crdt.NewCRDTStatsStore(ctx, broadcaster, "node1")
require.NoError(t, err)
defer statsStore.Close()

// Create CRDT like repository
likeRepo := NewCRDTLikeRepo(db, statsStore)

// Test increment likes
tweetID := "tweet123"

err = likeRepo.IncrementLikes(tweetID)
require.NoError(t, err)

// Test get likes count
count, err := likeRepo.GetLikesCount(tweetID)
require.NoError(t, err)
require.Equal(t, uint64(1), count)

// Test decrement likes
err = likeRepo.DecrementLikes(tweetID)
require.NoError(t, err)

count, err = likeRepo.GetLikesCount(tweetID)
require.NoError(t, err)
require.Equal(t, uint64(0), count)
}

func TestCRDTTweetRepo(t *testing.T) {
if testing.Short() {
t.Skip("skipping test in short mode")
}

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// Create mock broadcaster
broadcaster := newMockBroadcaster()

// Create CRDT stats store
statsStore, err := crdt.NewCRDTStatsStore(ctx, broadcaster, "node1")
require.NoError(t, err)
defer statsStore.Close()

// Create CRDT tweet repository
tweetRepo := NewCRDTTweetRepo(statsStore)

tweetID := "tweet789"

// Test increment retweets
err = tweetRepo.IncrementRetweets(tweetID)
require.NoError(t, err)

// Test get retweets count
count, err := tweetRepo.GetRetweetsCount(tweetID)
require.NoError(t, err)
require.Equal(t, uint64(1), count)

// Test decrement retweets
err = tweetRepo.DecrementRetweets(tweetID)
require.NoError(t, err)

count, err = tweetRepo.GetRetweetsCount(tweetID)
require.NoError(t, err)
require.Equal(t, uint64(0), count)

// Test views
err = tweetRepo.IncrementViews(tweetID)
require.NoError(t, err)

viewCount, err := tweetRepo.GetViewsCount(tweetID)
require.NoError(t, err)
require.Equal(t, uint64(1), viewCount)
}

func TestCRDTReplyRepo(t *testing.T) {
if testing.Short() {
t.Skip("skipping test in short mode")
}

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// Create mock broadcaster
broadcaster := newMockBroadcaster()

// Create CRDT stats store
statsStore, err := crdt.NewCRDTStatsStore(ctx, broadcaster, "node1")
require.NoError(t, err)
defer statsStore.Close()

// Create CRDT reply repository
replyRepo := NewCRDTReplyRepo(statsStore)

rootID := "tweet123"

// Test increment replies
err = replyRepo.IncrementReplies(rootID)
require.NoError(t, err)

// Test get replies count
count, err := replyRepo.GetRepliesCount(rootID)
require.NoError(t, err)
require.Equal(t, uint64(1), count)

// Test decrement replies
err = replyRepo.DecrementReplies(rootID)
require.NoError(t, err)

count, err = replyRepo.GetRepliesCount(rootID)
require.NoError(t, err)
require.Equal(t, uint64(0), count)
}
