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
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubPollRepo struct {
	voteFn    func(tweetId, userId string, option int, isTransitive bool) error
	votedFn   func(tweetId, userId string) (int, bool, error)
	resultsFn func(tweetId string, optionsNum int) ([]uint64, error)
}

func (s stubPollRepo) Vote(tweetId, userId string, option int, isTransitive bool) error {
	if s.voteFn != nil {
		return s.voteFn(tweetId, userId, option, isTransitive)
	}
	return nil
}

func (s stubPollRepo) Voted(tweetId, userId string) (int, bool, error) {
	if s.votedFn != nil {
		return s.votedFn(tweetId, userId)
	}
	return 0, false, nil
}

func (s stubPollRepo) Results(tweetId string, optionsNum int) ([]uint64, error) {
	if s.resultsFn != nil {
		return s.resultsFn(tweetId, optionsNum)
	}
	return make([]uint64, optionsNum), nil
}

type stubPollTweetRepo struct {
	getFn func(userId, tweetId string) (domain.Tweet, error)
}

func (s stubPollTweetRepo) Get(userId, tweetId string) (domain.Tweet, error) {
	if s.getFn != nil {
		return s.getFn(userId, tweetId)
	}
	return domain.Tweet{}, database.ErrTweetNotFound
}

func pollTweet(tweetId, authorId string, expiresAt time.Time) stubPollTweetRepo {
	return stubPollTweetRepo{getFn: func(_, _ string) (domain.Tweet, error) {
		return domain.Tweet{
			Id:     tweetId,
			UserId: authorId,
			Poll:   &domain.Poll{Options: []string{"yes", "no"}, ExpiresAt: expiresAt},
		}, nil
	}}
}

func TestStreamPollVoteHandler(t *testing.T) {
	voter := "owner-1"
	author := "tweet-owner"
	tweetId := "tweet-1"
	future := time.Now().Add(time.Hour)

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner id", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, UserId: author, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: empty owner id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{OwnerId: voter, UserId: author, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("negative option", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: -1, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: negative option" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("option out of range", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, pollTweet(tweetId, author, future), stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 2, OptionsNum: 3}), nil)
		if err == nil || err.Error() != "poll: option out of range" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("closed poll", func(t *testing.T) {
		h := StreamPollVoteHandler(stubPollRepo{}, pollTweet(tweetId, author, time.Now().Add(-time.Hour)), stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 1, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: poll is closed" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("vote repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		repo := stubPollRepo{voteFn: func(_, _ string, _ int, _ bool) error { return repoErr }}
		h := StreamPollVoteHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 1, OptionsNum: 2}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("happy path bumps the crdt counter on the voter's own node", func(t *testing.T) {
		var gotTransitive bool
		repo := stubPollRepo{
			voteFn: func(_, gotUser string, gotOption int, isTransitive bool) error {
				gotTransitive = isTransitive
				if gotUser != voter {
					t.Fatalf("expected the voter to be recorded, got %q", gotUser)
				}
				if gotOption != 1 {
					t.Fatalf("unexpected option: %d", gotOption)
				}
				return nil
			},
			resultsFn: func(_ string, optionsNum int) ([]uint64, error) {
				return []uint64{3, 4}, nil
			},
			votedFn: func(_, _ string) (int, bool, error) { return 1, true, nil },
		}
		streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: voter}}
		h := StreamPollVoteHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 1, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !gotTransitive {
			t.Fatal("expected the vote to be transitive on the voter's own node")
		}
		out := resp.(event.PollResultsResponse)
		if out.TotalVotes != 7 {
			t.Fatalf("unexpected total: %d", out.TotalVotes)
		}
		if out.VotedOption == nil || *out.VotedOption != 1 {
			t.Fatalf("unexpected voted option: %v", out.VotedOption)
		}
	})

	t.Run("vote received on the author's node is not transitive", func(t *testing.T) {
		var gotTransitive = true
		repo := stubPollRepo{voteFn: func(_, _ string, _ int, isTransitive bool) error {
			gotTransitive = isTransitive
			return nil
		}}
		streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}}
		h := StreamPollVoteHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		if _, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 0, OptionsNum: 2}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if gotTransitive {
			t.Fatal("expected the vote not to be transitive on the author's node")
		}
	})

	t.Run("propagates to the author's node", func(t *testing.T) {
		var gotPath stream.WarpRoute
		var gotNode string
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: voter},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, _ any) ([]byte, error) {
				gotNode, gotPath = nodeId, path
				return nil, nil
			},
		}
		h := StreamPollVoteHandler(stubPollRepo{}, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		if _, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 0, OptionsNum: 2}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if gotNode != "node-2" || gotPath != event.PUBLIC_POST_POLL_VOTE {
			t.Fatalf("unexpected propagation: %s %s", gotNode, gotPath)
		}
	})

	t.Run("offline author still returns local results", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: voter},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		}
		repo := stubPollRepo{resultsFn: func(string, int) ([]uint64, error) { return []uint64{1, 0}, nil }}
		h := StreamPollVoteHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 0, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.PollResultsResponse).TotalVotes != 1 {
			t.Fatalf("unexpected total: %v", resp)
		}
	})

	t.Run("a replayed vote is not an error", func(t *testing.T) {
		streamed := false
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: voter},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				streamed = true
				return nil, nil
			},
		}
		repo := stubPollRepo{
			voteFn:    func(string, string, int, bool) error { return database.ErrPollAlreadyVoted },
			resultsFn: func(string, int) ([]uint64, error) { return []uint64{2, 5}, nil },
		}
		h := StreamPollVoteHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 0, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.PollResultsResponse).TotalVotes != 7 {
			t.Fatalf("unexpected total: %v", resp)
		}
		if streamed {
			t.Fatal("a replayed vote must not be propagated again")
		}
	})

	t.Run("unknown tweet trusts the payload's option count", func(t *testing.T) {
		var gotOptionsNum int
		repo := stubPollRepo{resultsFn: func(_ string, optionsNum int) ([]uint64, error) {
			gotOptionsNum = optionsNum
			return make([]uint64, optionsNum), nil
		}}
		streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: voter}}
		h := StreamPollVoteHandler(repo, stubPollTweetRepo{}, stubReactionUserRepo{}, streamer)

		if _, err := h(marshal(t, event.PollVoteEvent{TweetId: tweetId, OwnerId: voter, UserId: author, Option: 2, OptionsNum: 4}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if gotOptionsNum != 4 {
			t.Fatalf("unexpected options num: %d", gotOptionsNum)
		}
	})
}

func TestStreamGetPollHandler(t *testing.T) {
	reader := "owner-1"
	author := "tweet-owner"
	tweetId := "tweet-1"
	future := time.Now().Add(time.Hour)

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetPollHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetPollHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetPollEvent{TweetId: tweetId, OwnerId: reader, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamGetPollHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetPollEvent{UserId: author, OwnerId: reader, OptionsNum: 2}), nil)
		if err == nil || err.Error() != "poll: empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty options number", func(t *testing.T) {
		h := StreamGetPollHandler(stubPollRepo{}, stubPollTweetRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.GetPollEvent{TweetId: tweetId, UserId: author, OwnerId: reader}), nil)
		if err == nil || err.Error() != "poll: empty options number" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("local read on the author's own node", func(t *testing.T) {
		repo := stubPollRepo{
			resultsFn: func(_ string, optionsNum int) ([]uint64, error) { return []uint64{1, 2}, nil },
			votedFn:   func(_, _ string) (int, bool, error) { return 0, true, nil },
		}
		streamer := stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}}
		h := StreamGetPollHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.GetPollEvent{TweetId: tweetId, UserId: author, OwnerId: author, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := resp.(event.PollResultsResponse)
		if out.TotalVotes != 3 {
			t.Fatalf("unexpected total: %d", out.TotalVotes)
		}
		if out.VotedOption == nil || *out.VotedOption != 0 {
			t.Fatalf("unexpected voted option: %v", out.VotedOption)
		}
	})

	t.Run("forwarded read keeps the local voted option", func(t *testing.T) {
		remote, err := json.Marshal(event.PollResultsResponse{
			TweetId: tweetId, Votes: []uint64{9, 1}, TotalVotes: 10,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		streamer := stubStreamer{
			nodeInfo:        warpnet.NodeInfo{OwnerId: reader},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) { return remote, nil },
		}
		repo := stubPollRepo{
			resultsFn: func(string, int) ([]uint64, error) { return []uint64{0, 1}, nil },
			votedFn:   func(_, _ string) (int, bool, error) { return 1, true, nil },
		}
		h := StreamGetPollHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.GetPollEvent{TweetId: tweetId, UserId: author, OwnerId: reader, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		out := resp.(event.PollResultsResponse)
		if out.TotalVotes != 10 {
			t.Fatalf("expected the author's tally, got: %d", out.TotalVotes)
		}
		if out.VotedOption == nil || *out.VotedOption != 1 {
			t.Fatalf("unexpected voted option: %v", out.VotedOption)
		}
	})

	t.Run("offline author falls back to the local tally", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: reader},
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		}
		repo := stubPollRepo{resultsFn: func(string, int) ([]uint64, error) { return []uint64{0, 1}, nil }}
		h := StreamGetPollHandler(repo, pollTweet(tweetId, author, future), stubReactionUserRepo{}, streamer)

		resp, err := h(marshal(t, event.GetPollEvent{TweetId: tweetId, UserId: author, OwnerId: reader, OptionsNum: 2}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.PollResultsResponse).TotalVotes != 1 {
			t.Fatalf("unexpected total: %v", resp)
		}
	})
}
