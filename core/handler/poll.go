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

package handler

import (
	"errors"
	"strings"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type PollVotesStorer interface {
	Vote(tweetId, userId string, option int, isTransitive bool) error
	Voted(tweetId, userId string) (option int, ok bool, err error)
	Results(tweetId string, optionsNum int) (votes []uint64, err error)
}

// PollTweetFetcher resolves the tweet carrying the poll definition. The
// definition is the only place the option count and deadline live, so it is
// what bounds a vote.
type PollTweetFetcher interface {
	Get(userID, tweetID string) (tweet domain.Tweet, err error)
}

type PollStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type PollUserFetcher interface {
	Get(userId string) (user domain.User, err error)
}

func StreamPollVoteHandler(
	repo PollVotesStorer,
	tweetRepo PollTweetFetcher,
	userRepo PollUserFetcher,
	streamer PollStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.PollVoteEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.OwnerId == "" {
			return nil, warpnet.WarpError("poll: empty owner id")
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("poll: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("poll: empty tweet id")
		}
		if ev.Option < 0 {
			return nil, warpnet.WarpError("poll: negative option")
		}

		voter, _ := userRepo.Get(ev.OwnerId)
		if err := warpnet.VerifyAuthorship(s, voter.NodeId); err != nil {
			return nil, err
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		optionsNum := ev.OptionsNum

		// The poll definition bounds the vote, but only a node holding the
		// tweet can see it. Elsewhere the vote is taken on trust — the
		// author's node, which always holds it, is the one that rejects an
		// out-of-range or late vote.
		if poll, ok := localPoll(tweetRepo, ev.UserId, tweetId); ok {
			if ev.Option >= len(poll.Options) {
				return nil, warpnet.WarpError("poll: option out of range")
			}
			if poll.IsClosed() {
				return nil, warpnet.WarpError("poll: poll is closed")
			}
			optionsNum = len(poll.Options)
		}

		ownNodeInfo := streamer.NodeInfo()
		// Mirrors the reaction path: the network-wide (CRDT) counter is bumped
		// only on the voter's own node, so a vote stored on both the voter's
		// and the author's node is counted once.
		err := repo.Vote(tweetId, ev.OwnerId, ev.Option, ev.OwnerId == ownNodeInfo.OwnerId)
		if err != nil && !errors.Is(err, database.ErrPollAlreadyVoted) {
			log.Errorf("poll vote handler failed: %v", err)
			return nil, err
		}
		// An already-recorded vote is not a failure: the event was replayed,
		// or it reached this node twice. Report the current state.
		alreadyVoted := err != nil

		if !alreadyVoted && ev.OwnerId != ev.UserId {
			propagateVote(ev, userRepo, streamer, ownNodeInfo)
		}

		return pollResults(repo, tweetId, ev.OwnerId, optionsNum)
	}
}

// StreamGetPollHandler serves the vote tallies for a tweet's poll. Reads are
// forwarded to the author's node — it sees every vote — while "did I vote"
// is answered from this node, which is where the local user's own vote is
// recorded even when the author is offline.
func StreamGetPollHandler(
	repo PollVotesStorer,
	tweetRepo PollTweetFetcher,
	userRepo PollUserFetcher,
	streamer PollStreamer,
) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetPollEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("poll: empty user id")
		}
		if ev.TweetId == "" {
			return nil, warpnet.WarpError("poll: empty tweet id")
		}

		tweetId := strings.TrimPrefix(ev.TweetId, domain.RetweetPrefix)
		optionsNum := ev.OptionsNum
		if poll, ok := localPoll(tweetRepo, ev.UserId, tweetId); ok {
			optionsNum = len(poll.Options)
		}
		if optionsNum <= 0 {
			return nil, warpnet.WarpError("poll: empty options number")
		}

		if remote, ok := forwardPollRead(ev, userRepo, streamer); ok {
			remote.VotedOption = localVotedOption(repo, tweetId, ev.OwnerId)
			return remote, nil
		}

		return pollResults(repo, tweetId, ev.OwnerId, optionsNum)
	}
}

// localPoll returns the poll definition of a locally stored tweet. ok=false
// means this node doesn't hold the tweet, or it carries no poll.
func localPoll(tweetRepo PollTweetFetcher, authorId, tweetId string) (domain.Poll, bool) {
	if tweetRepo == nil {
		return domain.Poll{}, false
	}
	tweet, err := tweetRepo.Get(authorId, tweetId)
	if err != nil || tweet.Poll == nil || len(tweet.Poll.Options) == 0 {
		return domain.Poll{}, false
	}
	return *tweet.Poll, true
}

// propagateVote forwards the vote to the tweet author's node so the tally
// everyone reads converges. An offline author is not a failure — the vote is
// already recorded here and the CRDT counter carries it across.
func propagateVote(
	ev event.PollVoteEvent,
	userRepo PollUserFetcher,
	streamer PollStreamer,
	ownNodeInfo warpnet.NodeInfo,
) {
	if ev.UserId == ownNodeInfo.OwnerId {
		return // this node hosts the author: the vote is already where it belongs
	}
	author, err := userRepo.Get(ev.UserId)
	if errors.Is(err, database.ErrUserNotFound) {
		return
	}
	if err != nil {
		log.Errorf("poll vote: get author: %v", err)
		return
	}
	if author.NodeId == ownNodeInfo.ID.String() {
		return
	}

	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_POST_POLL_VOTE, ev)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		return
	}
	if err != nil {
		log.Errorf("poll vote: stream to %s: %v", author.NodeId, err)
		return
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		log.Errorf("unmarshal other poll vote error response: %s", possibleError.Message)
	}
}

// forwardPollRead asks the author's node for the tally. ok=false means
// handle the read locally.
func forwardPollRead(
	ev event.GetPollEvent,
	userRepo PollUserFetcher,
	streamer PollStreamer,
) (event.PollResultsResponse, bool) {
	ownNodeInfo := streamer.NodeInfo()
	if ev.UserId == ownNodeInfo.OwnerId {
		return event.PollResultsResponse{}, false
	}
	author, err := userRepo.Get(ev.UserId)
	if err != nil || author.NodeId == "" || author.NodeId == ownNodeInfo.ID.String() {
		return event.PollResultsResponse{}, false
	}

	resp, err := streamer.GenericStream(author.NodeId, event.PUBLIC_GET_POLL, ev)
	if err != nil {
		if !errors.Is(err, warpnet.ErrNodeIsOffline) {
			log.Errorf("poll read: stream to %s: %v", author.NodeId, err)
		}
		return event.PollResultsResponse{}, false
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(resp, &possibleError); possibleError.Message != "" {
		log.Errorf("unmarshal other poll read error response: %s", possibleError.Message)
		return event.PollResultsResponse{}, false
	}

	var out event.PollResultsResponse
	if err := json.Unmarshal(resp, &out); err != nil || len(out.Votes) == 0 {
		return event.PollResultsResponse{}, false
	}
	return out, true
}

// localVotedOption reports the option userId picked, as recorded on this
// node. nil means no local record of a vote.
func localVotedOption(repo PollVotesStorer, tweetId, userId string) *int {
	if userId == "" {
		return nil
	}
	option, ok, err := repo.Voted(tweetId, userId)
	if err != nil {
		log.Warnf("poll: voted lookup: %v", err)
		return nil
	}
	if !ok {
		return nil
	}
	return &option
}

func pollResults(repo PollVotesStorer, tweetId, userId string, optionsNum int) (event.PollResultsResponse, error) {
	if optionsNum <= 0 {
		return event.PollResultsResponse{}, warpnet.WarpError("poll: empty options number")
	}
	votes, err := repo.Results(tweetId, optionsNum)
	if err != nil {
		return event.PollResultsResponse{}, err
	}
	var total uint64
	for _, v := range votes {
		total += v
	}
	return event.PollResultsResponse{
		TweetId:     tweetId,
		Votes:       votes,
		TotalVotes:  total,
		VotedOption: localVotedOption(repo, tweetId, userId),
	}, nil
}
