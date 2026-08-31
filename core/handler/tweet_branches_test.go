//nolint:all
package handler

import (
	"crypto/ed25519"
	"crypto/rand"
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

// nodeIdentity mints a real libp2p peer id plus a loopback stream whose remote
// peer is that id, so handlers guarded by VerifyAuthorship accept the caller.
func nodeIdentity(t *testing.T) (warpnet.WarpPeerID, warpnet.WarpStream) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	_, server := stream.NewLoopbackStream(id, id, "/test/route/0.0.0")
	return id, server
}

func TestStreamPinUnpinTweetHandler(t *testing.T) {
	nodeId, s := nodeIdentity(t)
	const userId, tweetId = "user-1", "tweet-1"

	authorRepo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: nodeId.String()}, nil
	}}

	t.Run("pins", func(t *testing.T) {
		out, err := StreamPinTweetHandler(stubTweetRepo{}, authorRepo)(
			marshal(t, event.PinTweetEvent{UserId: userId, TweetId: tweetId}), s)
		require.NoError(t, err)
		require.True(t, out.(domain.Tweet).Pinned)
	})

	t.Run("unpins", func(t *testing.T) {
		out, err := StreamUnpinTweetHandler(stubTweetRepo{}, authorRepo)(
			marshal(t, event.PinTweetEvent{UserId: userId, TweetId: tweetId}), s)
		require.NoError(t, err)
		require.False(t, out.(domain.Tweet).Pinned)
	})

	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamPinTweetHandler(stubTweetRepo{}, authorRepo)([]byte("{"), s)
		require.Error(t, err)
	})

	t.Run("empty ids", func(t *testing.T) {
		_, err := StreamPinTweetHandler(stubTweetRepo{}, authorRepo)(
			marshal(t, event.PinTweetEvent{TweetId: tweetId}), s)
		require.Error(t, err)

		_, err = StreamUnpinTweetHandler(stubTweetRepo{}, authorRepo)(
			marshal(t, event.PinTweetEvent{UserId: userId}), s)
		require.Error(t, err)
	})

	t.Run("foreign author is rejected", func(t *testing.T) {
		foreign := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "somebody-else"}, nil
		}}
		_, err := StreamPinTweetHandler(stubTweetRepo{}, foreign)(
			marshal(t, event.PinTweetEvent{UserId: userId, TweetId: tweetId}), s)
		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
	})

	t.Run("missing tweet", func(t *testing.T) {
		repoErr := errors.New("gone")
		_, err := StreamPinTweetHandler(stubTweetRepo{getFn: func(_, _ string) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}, authorRepo)(marshal(t, event.PinTweetEvent{UserId: userId, TweetId: tweetId}), s)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("someone else's tweet", func(t *testing.T) {
		_, err := StreamPinTweetHandler(stubTweetRepo{getFn: func(_, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, UserId: "another-user"}, nil
		}}, authorRepo)(marshal(t, event.PinTweetEvent{UserId: userId, TweetId: tweetId}), s)
		require.Error(t, err)
	})
}

func TestStreamEditTweetHandlerBranches(t *testing.T) {
	const userId, tweetId = "user-1", "tweet-1"

	t.Run("no-op edit skips the revision", func(t *testing.T) {
		repo := stubTweetRepo{getFn: func(u, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, UserId: u, Text: "same"}, nil
		}}
		out, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "same"}), nil)
		require.NoError(t, err)
		require.Equal(t, "same", domain.Tweet(out.(event.EditTweetResponse)).Text)
	})

	t.Run("only the author may edit", func(t *testing.T) {
		repo := stubTweetRepo{getFn: func(_, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, UserId: "another-user", Text: "old"}, nil
		}}
		_, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.Error(t, err)
	})

	t.Run("missing tweet", func(t *testing.T) {
		repoErr := errors.New("gone")
		repo := stubTweetRepo{getFn: func(_, _ string) (domain.Tweet, error) {
			return domain.Tweet{}, repoErr
		}}
		_, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.ErrorIs(t, err, repoErr)
	})

	t.Run("timeline refresh failure is tolerated", func(t *testing.T) {
		repo := stubTweetRepo{getFn: func(u, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, UserId: u, Text: "old"}, nil
		}}
		timeline := stubTimelineRepo{addFn: func(_ string, _ domain.Tweet) error {
			return errors.New("timeline down")
		}}
		_, err := StreamEditTweetHandler(repo, timeline)(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.NoError(t, err)
	})

	t.Run("nil timeline repo", func(t *testing.T) {
		repo := stubTweetRepo{getFn: func(u, id string) (domain.Tweet, error) {
			return domain.Tweet{Id: id, UserId: u, Text: "old"}, nil
		}}
		_, err := StreamEditTweetHandler(repo, nil)(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.NoError(t, err)
	})

	t.Run("cancels retweets of the edited tweet", func(t *testing.T) {
		var cancelled []string
		repo := stubTweetRepo{
			getFn: func(u, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, UserId: u, Text: "old"}, nil
			},
			unRetweetFn: func(by, id string) error {
				cancelled = append(cancelled, by)
				return nil
			},
			retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
				return []string{"rt-1", "rt-2"}, "", nil
			},
		}
		_, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.NoError(t, err)
		require.Equal(t, []string{"rt-1", "rt-2"}, cancelled)
	})

	t.Run("retweeter listing failure is tolerated", func(t *testing.T) {
		repo := stubTweetRepo{
			getFn: func(u, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, UserId: u, Text: "old"}, nil
			},
			retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
				return nil, "", errors.New("index down")
			},
		}
		_, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.NoError(t, err)
	})

	t.Run("unretweet failure is tolerated", func(t *testing.T) {
		repo := stubTweetRepo{
			getFn: func(u, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, UserId: u, Text: "old"}, nil
			},
			unRetweetFn: func(_, _ string) error { return errors.New("stuck") },
			retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
				return []string{"rt-1"}, "", nil
			},
		}
		_, err := StreamEditTweetHandler(repo, stubTimelineRepo{})(
			marshal(t, event.EditTweetEvent{UserId: userId, TweetId: tweetId, Text: "new"}), nil)
		require.NoError(t, err)
	})
}

func TestOwnReaction(t *testing.T) {
	t.Run("no owner", func(t *testing.T) {
		require.Empty(t, ownReaction(stubTweetReactionRepo{}, "tweet-1", ""))
	})

	t.Run("lookup failure is empty", func(t *testing.T) {
		repo := stubTweetReactionRepo{reactionFn: func(_, _ string) (string, error) {
			return "", errors.New("down")
		}}
		require.Empty(t, ownReaction(repo, "tweet-1", "owner-1"))
	})

	t.Run("returns the emoji", func(t *testing.T) {
		repo := stubTweetReactionRepo{reactionFn: func(_, _ string) (string, error) {
			return "👍", nil
		}}
		require.Equal(t, "👍", ownReaction(repo, "tweet-1", "owner-1"))
	})
}

func TestForwardThreadReplies(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId}

	t.Run("no root user means handle locally", func(t *testing.T) {
		_, ok := forwardThreadReplies(stubTweetUserRepo{}, stubStreamer{nodeInfo: selfInfo}, event.GetAllTweetsEvent{})
		require.False(t, ok)
	})

	t.Run("unknown author means handle locally", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		_, ok := forwardThreadReplies(repo, stubStreamer{nodeInfo: selfInfo}, event.GetAllTweetsEvent{RootUserId: "u"})
		require.False(t, ok)
	})

	t.Run("author on this node means handle locally", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		_, ok := forwardThreadReplies(repo, stubStreamer{nodeInfo: selfInfo}, event.GetAllTweetsEvent{RootUserId: "u"})
		require.False(t, ok)
	})

	t.Run("stream failure means handle locally", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "other-node"}, nil
		}}
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("offline")
		}}
		_, ok := forwardThreadReplies(repo, streamer, event.GetAllTweetsEvent{RootUserId: "u"})
		require.False(t, ok)
	})

	t.Run("garbage response means handle locally", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "other-node"}, nil
		}}
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		_, ok := forwardThreadReplies(repo, streamer, event.GetAllTweetsEvent{RootUserId: "u"})
		require.False(t, ok)
	})

	t.Run("forwards", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "other-node"}, nil
		}}
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.TweetsResponse{UserId: "u", Tweets: []domain.Tweet{{Id: "t1"}}})
		}}
		resp, ok := forwardThreadReplies(repo, streamer, event.GetAllTweetsEvent{RootUserId: "u"})
		require.True(t, ok)
		require.Len(t, resp.Tweets, 1)
	})
}

func TestTweetsRefreshBackground(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}
	ev := event.GetAllTweetsEvent{UserId: "other-user"}

	t.Run("unknown user is skipped", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		tweetsRefreshBackground(stubTweetRepo{}, repo, ev, stubStreamer{nodeInfo: selfInfo})
	})

	t.Run("lookup failure is skipped", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("down")
		}}
		tweetsRefreshBackground(stubTweetRepo{}, repo, ev, stubStreamer{nodeInfo: selfInfo})
	})

	t.Run("own node is skipped", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		tweetsRefreshBackground(stubTweetRepo{}, repo, ev, stubStreamer{nodeInfo: selfInfo})
	})

	otherNodeRepo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "other-node"}, nil
	}}

	t.Run("stream failure is skipped", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("offline")
		}}
		tweetsRefreshBackground(stubTweetRepo{}, otherNodeRepo, ev, streamer)
	})

	t.Run("error response is skipped", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.ResponseError{Message: "boom"})
		}}
		tweetsRefreshBackground(stubTweetRepo{}, otherNodeRepo, ev, streamer)
	})

	t.Run("garbage response is skipped", func(t *testing.T) {
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		tweetsRefreshBackground(stubTweetRepo{}, otherNodeRepo, ev, streamer)
	})

	t.Run("caches fetched tweets and skips blocklisted ones", func(t *testing.T) {
		var cached []string
		repo := stubTweetRepo{
			isBlocklistedFn: func(id string) bool { return id == "blocked" },
			createWithTTLFn: func(_ string, tw domain.Tweet, _ time.Duration) (domain.Tweet, error) {
				cached = append(cached, tw.Id)
				return tw, nil
			},
		}
		streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(event.TweetsResponse{Tweets: []domain.Tweet{
				{Id: "ok", UserId: "other-user"},
				{Id: "blocked", UserId: "other-user"},
			}})
		}}
		tweetsRefreshBackground(repo, otherNodeRepo, ev, streamer)
		require.Equal(t, []string{"ok"}, cached)
	})
}

func TestStreamGetTweetsHandlerRefreshesWhenEmpty(t *testing.T) {
	nodeId, _ := nodeIdentity(t)
	selfInfo := warpnet.NodeInfo{ID: nodeId, OwnerId: "owner-1"}

	calls := 0
	repo := stubTweetRepo{listFn: func(userId string, _ *uint64, _ *string) ([]domain.Tweet, string, error) {
		calls++
		if calls == 1 {
			return nil, "", nil // cold cache forces the synchronous refresh
		}
		return []domain.Tweet{{Id: "t1", UserId: userId}}, "cursor-1", nil
	}}
	userRepo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "other-node"}, nil
	}}
	streamer := stubStreamer{nodeInfo: selfInfo, genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
		return json.Marshal(event.TweetsResponse{Tweets: []domain.Tweet{{Id: "t1", UserId: "other-user"}}})
	}}

	out, err := StreamGetTweetsHandler(repo, userRepo, streamer)(
		marshal(t, event.GetAllTweetsEvent{UserId: "other-user"}), nil)
	require.NoError(t, err)

	resp := out.(event.TweetsResponse)
	require.Len(t, resp.Tweets, 1)
	require.Equal(t, "cursor-1", resp.Cursor)
}

func TestStreamGetTweetHandlerFallsBackToLocal(t *testing.T) {
	const owner, other = "owner-1", "other-user"
	auth := stubAuth{owner: domain.Owner{UserId: owner}}
	otherNodeRepo := stubTweetUserRepo{getFn: func(id string) (domain.User, error) {
		return domain.User{Id: id, NodeId: "other-node"}, nil
	}}
	ev := event.GetTweetEvent{UserId: other, TweetId: "t1"}

	t.Run("user lookup failure", func(t *testing.T) {
		repo := stubTweetUserRepo{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("down")
		}}
		_, err := StreamGetTweetHandler(stubTweetRepo{}, auth, repo, stubStreamer{})(marshal(t, ev), nil)
		require.Error(t, err)
	})

	t.Run("stream failure", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, errors.New("boom")
		}}
		_, err := StreamGetTweetHandler(stubTweetRepo{}, auth, otherNodeRepo, streamer)(marshal(t, ev), nil)
		require.Error(t, err)
	})

	t.Run("garbage response falls back to the local copy", func(t *testing.T) {
		streamer := stubStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		out, err := StreamGetTweetHandler(stubTweetRepo{}, auth, otherNodeRepo, streamer)(marshal(t, ev), nil)
		require.NoError(t, err)
		require.Equal(t, "cached", out.(domain.Tweet).Text)
	})
}

func TestStreamDeleteTweetHandlerBranches(t *testing.T) {
	const owner, tweetId = "owner-1", "tweet-1"

	t.Run("unreact failure is tolerated", func(t *testing.T) {
		reactions := stubTweetReactionRepo{unreactFn: func(_, _ string) (uint64, error) {
			return 0, errors.New("down")
		}}
		out, err := StreamDeleteTweetHandler(
			stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{}, stubTimelineRepo{}, reactions, stubStreamer{},
		)(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		require.NoError(t, err)
		require.Equal(t, event.Accepted, out)
	})

	t.Run("retweet record is unwound", func(t *testing.T) {
		by := "retweeter-1"
		var unretweeted string
		repo := stubTweetRepo{
			getFn: func(_, id string) (domain.Tweet, error) {
				return domain.Tweet{Id: id, RetweetedBy: &by}, nil
			},
			unRetweetFn: func(byUser, _ string) error {
				unretweeted = byUser
				return nil
			},
		}
		_, err := StreamDeleteTweetHandler(
			stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}},
			repo, stubTimelineRepo{}, stubTweetReactionRepo{}, stubStreamer{},
		)(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		require.NoError(t, err)
		require.Equal(t, by, unretweeted)
	})

	t.Run("timeline delete failure is tolerated", func(t *testing.T) {
		timeline := stubTimelineRepo{deleteFn: func(_, _ string) error { return errors.New("down") }}
		_, err := StreamDeleteTweetHandler(
			stubTweetBroadcaster{}, stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{}, timeline, stubTweetReactionRepo{}, stubStreamer{},
		)(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		require.NoError(t, err)
	})

	t.Run("broadcast failure is tolerated", func(t *testing.T) {
		broadcaster := stubTweetBroadcaster{publishFn: func(_, _ string, _ []byte) error {
			return errors.New("no followers channel")
		}}
		_, err := StreamDeleteTweetHandler(
			broadcaster, stubAuth{owner: domain.Owner{UserId: owner}},
			stubTweetRepo{}, stubTimelineRepo{}, stubTweetReactionRepo{}, stubStreamer{},
		)(marshal(t, event.DeleteTweetEvent{UserId: owner, TweetId: tweetId}), nil)
		require.NoError(t, err)
	})
}
