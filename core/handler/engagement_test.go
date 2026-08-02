//nolint:all
package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEngagementStreamer struct {
	info     warpnet.NodeInfo
	response []byte
	err      error

	calls []string
}

func (f *fakeEngagementStreamer) NodeInfo() warpnet.NodeInfo { return f.info }

func (f *fakeEngagementStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	f.calls = append(f.calls, nodeId+" "+string(path))
	return f.response, f.err
}

type stubLikersRepo struct {
	likersFn func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubLikersRepo) Likers(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.likersFn != nil {
		return s.likersFn(tweetId, limit, cursor)
	}
	return nil, "end", nil
}

type stubRetweetersRepo struct {
	retweetersFn func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubRetweetersRepo) Retweeters(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.retweetersFn != nil {
		return s.retweetersFn(tweetId, limit, cursor)
	}
	return nil, "end", nil
}

type stubLikedUserFetcher struct {
	batchFn func(ids ...string) ([]domain.User, error)
	getFn   func(id string) (domain.User, error)
}

func (s stubLikedUserFetcher) GetBatch(ids ...string) ([]domain.User, error) {
	if s.batchFn != nil {
		return s.batchFn(ids...)
	}
	out := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.User{Id: id})
	}
	return out, nil
}

func (s stubLikedUserFetcher) Get(id string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(id)
	}
	return domain.User{Id: id}, nil
}

func TestStreamGetTweetLikersHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{}, stubLikedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamGetTweetLikersHandler(stubLikersRepo{likersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{TweetId: "t"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path hydrates users", func(t *testing.T) {
		resp, err := StreamGetTweetLikersHandler(stubLikersRepo{likersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"u1", "u2"}, "end", nil
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetLikersEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(r.Users))
		}
		if r.Cursor != "end" {
			t.Fatalf("expected end cursor, got %s", r.Cursor)
		}
	})
}

func TestStreamGetTweetRetweetersHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubLikedUserFetcher{}, nil)([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty tweet id", func(t *testing.T) {
		_, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetTweetRetweetersHandler(stubRetweetersRepo{retweetersFn: func(_ string, _ *uint64, _ *string) ([]string, string, error) {
			return []string{"r1"}, "end", nil
		}}, stubLikedUserFetcher{}, nil)(marshal(t, event.GetTweetRetweetersEvent{TweetId: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.UsersResponse)
		if len(r.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(r.Users))
		}
	})
}

// ---------------------------------------------------------------------------
// Engagement forwarding — reading likers/retweeters off the author's node.
// ---------------------------------------------------------------------------

func TestForwardToOwner(t *testing.T) {
	ev := event.GetTweetLikersEvent{TweetId: "t1", OwnerUserId: "author"}

	t.Run("no owner or no streamer stays local", func(t *testing.T) {
		_, ok, err := forwardToOwner("", &fakeEngagementStreamer{}, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)

		_, ok, err = forwardToOwner("author", nil, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	// Asking ourselves over the network would be an infinite loop.
	t.Run("owner is this node's owner stays local", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "author"}}
		_, ok, err := forwardToOwner("author", streamer, stubLikedUserFetcher{}, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, streamer.calls, "a node must never stream to itself")
	})

	t.Run("unknown owner stays local", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "me"}}
		users := stubLikedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, streamer.calls)
	})

	t.Run("owner lookup error propagates", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{info: warpnet.NodeInfo{OwnerId: "me"}}
		users := stubLikedUserFetcher{getFn: func(string) (domain.User, error) {
			return domain.User{}, errors.New("db down")
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		assert.Error(t, err)
		assert.False(t, ok)
	})

	// An offline author must degrade to whatever this node already knows,
	// not surface an error to the reader.
	t.Run("offline owner degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			err:  warpnet.ErrNodeIsOffline,
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("other stream errors propagate", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			err:  errors.New("connection reset"),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		assert.Error(t, err)
		assert.False(t, ok)
	})

	// A malicious or simply outdated peer can answer with garbage — the reader
	// must still get this node's own view instead of an error.
	t.Run("unparseable remote answer degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info:     warpnet.NodeInfo{OwnerId: "me"},
			response: []byte("<html>not json</html>"),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("empty remote page degrades to the local index", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info:     warpnet.NodeInfo{OwnerId: "me"},
			response: mustJSON(t, event.UsersResponse{}),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		_, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("non-empty remote page wins", func(t *testing.T) {
		streamer := &fakeEngagementStreamer{
			info: warpnet.NodeInfo{OwnerId: "me"},
			response: mustJSON(t, event.UsersResponse{
				Cursor: "remote-cursor",
				Users:  []domain.User{{Id: "liker-1"}},
			}),
		}
		users := stubLikedUserFetcher{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "remote-node"}, nil
		}}
		out, ok, err := forwardToOwner("author", streamer, users, event.PUBLIC_GET_TWEET_LIKERS, ev)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "remote-cursor", out.Cursor)
		require.Len(t, out.Users, 1)
		assert.Equal(t, "liker-1", out.Users[0].Id)
		require.Len(t, streamer.calls, 1)
		assert.True(t, strings.HasPrefix(streamer.calls[0], "remote-node "))
	})
}

func TestHydrateUsers(t *testing.T) {
	t.Run("no ids yields nothing", func(t *testing.T) {
		assert.Nil(t, hydrateUsers(stubLikedUserFetcher{}, nil))
		assert.Nil(t, hydrateUsers(stubLikedUserFetcher{}, []string{}))
	})

	t.Run("batch result is used as-is", func(t *testing.T) {
		users := stubLikedUserFetcher{batchFn: func(ids ...string) ([]domain.User, error) {
			return []domain.User{{Id: "a"}, {Id: "b"}}, nil
		}}
		got := hydrateUsers(users, []string{"a", "b"})
		assert.Len(t, got, 2)
	})

	// A failing batch read must not blank the whole engagement list.
	t.Run("batch failure falls back to per-id reads", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) {
				return nil, errors.New("batch exploded")
			},
			getFn: func(id string) (domain.User, error) {
				return domain.User{Id: id, Username: "user-" + id}, nil
			},
		}
		got := hydrateUsers(users, []string{"a", "b"})
		require.Len(t, got, 2)
		assert.Equal(t, "user-a", got[0].Username)
	})

	// One deleted account must not erase everyone else from the list.
	t.Run("individually missing users are skipped not fatal", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) { return nil, nil },
			getFn: func(id string) (domain.User, error) {
				if id == "gone" {
					return domain.User{}, database.ErrUserNotFound
				}
				return domain.User{Id: id}, nil
			},
		}
		got := hydrateUsers(users, []string{"a", "gone", "b"})
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Id)
		assert.Equal(t, "b", got[1].Id)
	})

	t.Run("all users missing yields an empty list", func(t *testing.T) {
		users := stubLikedUserFetcher{
			batchFn: func(ids ...string) ([]domain.User, error) { return nil, errors.New("x") },
			getFn: func(string) (domain.User, error) {
				return domain.User{}, database.ErrUserNotFound
			},
		}
		assert.Empty(t, hydrateUsers(users, []string{"a", "b"}))
	})
}
