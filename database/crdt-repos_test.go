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
	"github.com/Warp-net/warpnet/domain"
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

	// Test like operation
	tweetID := "tweet123"
	userID := "user456"

	count, err := likeRepo.Like(tweetID, userID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test like count
	count, err = likeRepo.LikesCount(tweetID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test unlike operation
	count, err = likeRepo.Unlike(tweetID, userID)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)

	// Test likers
	_, err = likeRepo.Like(tweetID, userID)
	require.NoError(t, err)
	
	likers, _, err := likeRepo.Likers(tweetID, nil, nil)
	require.NoError(t, err)
	require.Len(t, likers, 1)
	require.Equal(t, userID, likers[0])
}

func TestCRDTTweetRepo(t *testing.T) {
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

	// Create CRDT tweet repository
	tweetRepo := NewCRDTTweetRepo(db, statsStore)

	// Create a tweet first
	tweet := domain.Tweet{
		Id:       "tweet789",
		UserId:   "user1",
		Username: "testuser",
		Text:     "Test tweet",
		RootId:   "tweet789",
	}

	created, err := tweetRepo.Create(tweet.UserId, tweet)
	require.NoError(t, err)
	require.NotEmpty(t, created.Id)

	// Test retweet
	retweeted, err := tweetRepo.NewRetweet(created)
	require.NoError(t, err)
	require.NotEmpty(t, retweeted.Id)

	// Test retweet count
	count, err := tweetRepo.RetweetsCount(created.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test unretweet
	err = tweetRepo.UnRetweet("user1", created.Id)
	require.NoError(t, err)

	count, err = tweetRepo.RetweetsCount(created.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)

	// Test view count
	viewCount, err := tweetRepo.IncrementViewCount(created.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), viewCount)

	viewCount, err = tweetRepo.GetViewCount(created.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), viewCount)
}

func TestCRDTReplyRepo(t *testing.T) {
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

	// Create CRDT reply repository
	replyRepo := NewCRDTReplyRepo(db, statsStore)

	// Create a reply
	rootID := "tweet123"
	parentID := "tweet123"
	reply := domain.Tweet{
		Id:       "reply456",
		RootId:   rootID,
		ParentId: &parentID,
		UserId:   "user1",
		Username: "testuser",
		Text:     "Test reply",
	}

	added, err := replyRepo.AddReply(reply)
	require.NoError(t, err)
	require.NotEmpty(t, added.Id)

	// Test reply count
	count, err := replyRepo.RepliesCount(rootID)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Test delete reply
	err = replyRepo.DeleteReply(rootID, parentID, added.Id)
	require.NoError(t, err)

	count, err = replyRepo.RepliesCount(rootID)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)
}
