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
	"github.com/stretchr/testify/require"
)

func TestLocalPoll(t *testing.T) {
	t.Run("nil repo", func(t *testing.T) {
		_, ok := localPoll(nil, "author", "tweet")
		require.False(t, ok)
	})

	t.Run("tweet not found", func(t *testing.T) {
		_, ok := localPoll(stubPollTweetRepo{}, "author", "tweet")
		require.False(t, ok)
	})

	t.Run("tweet without a poll", func(t *testing.T) {
		repo := stubPollTweetRepo{getFn: func(_, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id}, nil
		}}
		_, ok := localPoll(repo, "author", "tweet")
		require.False(t, ok)
	})

	t.Run("poll with no options", func(t *testing.T) {
		repo := stubPollTweetRepo{getFn: func(_, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, Poll: &domain.Poll{}}, nil
		}}
		_, ok := localPoll(repo, "author", "tweet")
		require.False(t, ok)
	})

	t.Run("poll present", func(t *testing.T) {
		poll, ok := localPoll(pollTweet("tweet", "author", time.Now().Add(time.Hour)), "author", "tweet")
		require.True(t, ok)
		require.Len(t, poll.Options, 2)
	})
}

func TestLocalVotedOption(t *testing.T) {
	t.Run("no user", func(t *testing.T) {
		require.Nil(t, localVotedOption(stubPollRepo{}, "tweet", ""))
	})

	t.Run("lookup failure", func(t *testing.T) {
		repo := stubPollRepo{votedFn: func(string, string) (int, bool, error) {
			return 0, false, errors.New("down")
		}}
		require.Nil(t, localVotedOption(repo, "tweet", "user"))
	})

	t.Run("no vote recorded", func(t *testing.T) {
		require.Nil(t, localVotedOption(stubPollRepo{}, "tweet", "user"))
	})

	t.Run("vote recorded", func(t *testing.T) {
		repo := stubPollRepo{votedFn: func(string, string) (int, bool, error) {
			return 1, true, nil
		}}
		got := localVotedOption(repo, "tweet", "user")
		require.NotNil(t, got)
		require.Equal(t, 1, *got)
	})
}

func TestPollResults(t *testing.T) {
	t.Run("rejects bad option counts", func(t *testing.T) {
		_, err := pollResults(stubPollRepo{}, "tweet", "user", 0)
		require.Error(t, err)
		_, err = pollResults(stubPollRepo{}, "tweet", "user", 21)
		require.Error(t, err)
	})

	t.Run("propagates the tally failure", func(t *testing.T) {
		repoErr := errors.New("down")
		repo := stubPollRepo{resultsFn: func(string, int) ([]uint64, error) { return nil, repoErr }}
		_, err := pollResults(repo, "tweet", "user", 2)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("sums the votes", func(t *testing.T) {
		repo := stubPollRepo{resultsFn: func(string, int) ([]uint64, error) {
			return []uint64{2, 3}, nil
		}}
		out, err := pollResults(repo, "tweet", "user", 2)
		require.NoError(t, err)
		require.Equal(t, uint64(5), out.TotalVotes)
	})
}

func TestPropagateVote(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}
	ev := event.PollVoteEvent{TweetId: "tweet-1", UserId: "author-1", OwnerId: "voter-1"}

	t.Run("author hosted here is skipped", func(t *testing.T) {
		propagateVote(event.PollVoteEvent{UserId: selfInfo.OwnerId}, stubUserFetcher{}, stubStreamer{}, selfInfo)
	})

	t.Run("unknown author is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		propagateVote(ev, repo, stubStreamer{}, selfInfo)
	})

	t.Run("lookup failure is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("down")
		}}
		propagateVote(ev, repo, stubStreamer{}, selfInfo)
	})

	t.Run("author on this node is skipped", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		propagateVote(ev, repo, stubStreamer{}, selfInfo)
	})

	otherNode := stubUserFetcher{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "other-node"}, nil
	}}

	t.Run("offline author is not a failure", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}}
		propagateVote(ev, otherNode, streamer, selfInfo)
	})

	t.Run("stream failure is swallowed", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("boom")
		}}
		propagateVote(ev, otherNode, streamer, selfInfo)
	})

	t.Run("remote error response is swallowed", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "rejected"})
		}}
		propagateVote(ev, otherNode, streamer, selfInfo)
	})
}

func TestForwardPollRead(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}
	ev := event.GetPollEvent{TweetId: "tweet-1", UserId: "author-1", OwnerId: "reader-1"}

	t.Run("own poll is read locally", func(t *testing.T) {
		_, ok := forwardPollRead(event.GetPollEvent{UserId: selfInfo.OwnerId}, stubUserFetcher{}, stubStreamer{nodeInfo: selfInfo})
		require.False(t, ok)
	})

	t.Run("unknown author is read locally", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		_, ok := forwardPollRead(ev, repo, stubStreamer{nodeInfo: selfInfo})
		require.False(t, ok)
	})

	t.Run("author on this node is read locally", func(t *testing.T) {
		repo := stubUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		_, ok := forwardPollRead(ev, repo, stubStreamer{nodeInfo: selfInfo})
		require.False(t, ok)
	})

	otherNode := stubUserFetcher{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "other-node"}, nil
	}}

	t.Run("offline author is read locally", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}}
		_, ok := forwardPollRead(ev, otherNode, streamer)
		require.False(t, ok)
	})

	t.Run("stream failure is read locally", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("boom")
		}}
		_, ok := forwardPollRead(ev, otherNode, streamer)
		require.False(t, ok)
	})

	t.Run("error response is read locally", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "no poll"})
		}}
		_, ok := forwardPollRead(ev, otherNode, streamer)
		require.False(t, ok)
	})

	t.Run("empty tally is read locally", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.PollResultsResponse{TweetId: "tweet-1"})
		}}
		_, ok := forwardPollRead(ev, otherNode, streamer)
		require.False(t, ok)
	})

	t.Run("remote tally wins", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.PollResultsResponse{TweetId: "tweet-1", Votes: []uint64{1, 2}, TotalVotes: 3})
		}}
		out, ok := forwardPollRead(ev, otherNode, streamer)
		require.True(t, ok)
		require.Equal(t, uint64(3), out.TotalVotes)
	})
}

func TestForwardViewToAuthor(t *testing.T) {
	ev := event.ViewEvent{TweetId: "tweet-1", UserId: "author-1", ViewerId: "viewer-1"}

	t.Run("unknown author counts nothing", func(t *testing.T) {
		repo := stubReactionUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		require.Zero(t, forwardViewToAuthor(ev, repo, stubStreamer{}))
	})

	t.Run("lookup failure counts nothing", func(t *testing.T) {
		repo := stubReactionUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("down")
		}}
		require.Zero(t, forwardViewToAuthor(ev, repo, stubStreamer{}))
	})

	t.Run("offline author counts nothing", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}}
		require.Zero(t, forwardViewToAuthor(ev, stubReactionUserRepo{}, streamer))
	})

	t.Run("stream failure counts nothing", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("boom")
		}}
		require.Zero(t, forwardViewToAuthor(ev, stubReactionUserRepo{}, streamer))
	})

	t.Run("error response counts nothing", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "nope"})
		}}
		require.Zero(t, forwardViewToAuthor(ev, stubReactionUserRepo{}, streamer))
	})

	t.Run("garbage response counts nothing", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		require.Zero(t, forwardViewToAuthor(ev, stubReactionUserRepo{}, streamer))
	})

	t.Run("author's count is returned", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ViewsCountResponse{Count: 7})
		}}
		require.Equal(t, uint64(7), forwardViewToAuthor(ev, stubReactionUserRepo{}, streamer))
	})
}

func TestStreamViewHandlerRecordFailure(t *testing.T) {
	const author, viewer, tweetId = "author-1", "viewer-1", "tweet-1"

	userRepo, s := actorStream(t, viewer, nil)
	repoErr := errors.New("counter down")
	repo := stubViewRepo{recordFn: func(string, string) (uint64, error) { return 0, repoErr }}

	h := StreamViewHandler(repo, userRepo, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: author}})
	_, err := h(marshal(t, event.ViewEvent{TweetId: tweetId, UserId: author, ViewerId: viewer}), s)
	require.ErrorIs(t, err, repoErr)
}
