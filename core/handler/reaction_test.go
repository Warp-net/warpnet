//nolint:all
package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubReactionRepo struct {
	reactFn          func(tweetId, userId, emoji string) (uint64, error)
	unreactFn        func(tweetId, userId string) (uint64, error)
	reactionsCountFn func(tweetId string) (uint64, error)
	reactorsFn       func(tweetId string, limit *uint64, cursor *string) ([]string, string, error)
	setReactedFn     func(userId, tweetId, ownerUserId string) error
	removeReactedFn  func(userId, tweetId string) error
	reactedFn        func(userId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error)
	reactionsFn      func(tweetId string) (map[string]uint64, error)
}

func (s stubReactionRepo) React(tweetId, userId, emoji string, _ bool) (uint64, error) {
	if s.reactFn != nil {
		return s.reactFn(tweetId, userId, emoji)
	}
	return 1, nil
}
func (s stubReactionRepo) Reactions(tweetId string) (map[string]uint64, error) {
	if s.reactionsFn != nil {
		return s.reactionsFn(tweetId)
	}
	return nil, nil
}
func (s stubReactionRepo) Unreact(tweetId, userId string, _ bool) (uint64, error) {
	if s.unreactFn != nil {
		return s.unreactFn(tweetId, userId)
	}
	return 0, nil
}
func (s stubReactionRepo) ReactionsCount(tweetId string) (uint64, error) {
	if s.reactionsCountFn != nil {
		return s.reactionsCountFn(tweetId)
	}
	return 0, nil
}
func (s stubReactionRepo) Reactors(tweetId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.reactorsFn != nil {
		return s.reactorsFn(tweetId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubReactionRepo) SetReacted(userId, tweetId, ownerUserId string) error {
	if s.setReactedFn != nil {
		return s.setReactedFn(userId, tweetId, ownerUserId)
	}
	return nil
}
func (s stubReactionRepo) RemoveReacted(userId, tweetId string) error {
	if s.removeReactedFn != nil {
		return s.removeReactedFn(userId, tweetId)
	}
	return nil
}
func (s stubReactionRepo) Reacted(userId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error) {
	if s.reactedFn != nil {
		return s.reactedFn(userId, limit, cursor)
	}
	return nil, "", nil
}

type stubReactionUserRepo struct {
	getBatchFn func(userIds ...string) ([]domain.User, error)
	getFn      func(userId string) (domain.User, error)
}

func (s stubReactionUserRepo) GetBatch(userIds ...string) ([]domain.User, error) {
	if s.getBatchFn != nil {
		return s.getBatchFn(userIds...)
	}
	return nil, nil
}
func (s stubReactionUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}

// actorStream builds a user repo and an inbound stream whose remote peer
// matches the NodeId the repo reports for actorId, so the authorship check
// passes for events claiming to come from actorId. Lookups of other user ids
// go to fallback, or to the stub default (NodeId "node-2") when fallback is nil.
func actorStream(t *testing.T, actorId string, fallback func(userId string) (domain.User, error)) (stubReactionUserRepo, warpnet.WarpStream) {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate actor key: %v", err)
	}
	nodeId, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive actor node id: %v", err)
	}

	repo := stubReactionUserRepo{getFn: func(userId string) (domain.User, error) {
		if userId == actorId {
			return domain.User{Id: userId, NodeId: nodeId.String(), Username: "actor"}, nil
		}
		if fallback != nil {
			return fallback(userId)
		}
		return domain.User{Id: userId, NodeId: "node-2"}, nil
	}}
	_, server := stream.NewLoopbackStream(nodeId, nodeId, "/test/route/0.0.0")
	return repo, server
}

func TestStreamReactionHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamReactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty owner id", func(t *testing.T) {
		h := StreamReactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "react: empty owner id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamReactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner}), nil)
		if err == nil || err.Error() != "react: empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamReactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.ReactionEvent{OwnerId: owner, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "react: empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("reaction repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{reactFn: func(tweetId, userId, emoji string) (uint64, error) {
			return 0, repoErr
		}}, users, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("foreign reactor is dropped", func(t *testing.T) {
		users, _ := actorStream(t, owner, nil)
		_, attacker := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), attacker)
		if !errors.Is(err, ErrForeignReactionAuthor) {
			t.Fatalf("expected foreign reaction author error, got: %v", err)
		}
	})

	t.Run("own tweet reaction", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("someone else reacted (exchange finished)", func(t *testing.T) {
		notified := false
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationReactionType {
				t.Fatalf("expected reaction type, got: %v", not.Type)
			}
			if not.RecepientId != tweetOwner {
				t.Fatalf("expected notification for tweet owner, got: %v", not.RecepientId)
			}
			return nil
		}}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner}})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("reacted user not found", func(t *testing.T) {
		users, conn := actorStream(t, owner, func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		})
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("user repo error", func(t *testing.T) {
		repoErr := errors.New("user repo")
		users, conn := actorStream(t, owner, func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		})
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected user repo error: %v", err)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("remote response with error payload", func(t *testing.T) {
		respErr, _ := json.Marshal(event.ResponseError{Code: 500, Message: "remote error"})
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return respErr, nil
			},
		})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("strips retweet prefix from tweet id", func(t *testing.T) {
		var capturedTweetId string
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{reactFn: func(tweetId, userId, emoji string) (uint64, error) {
			capturedTweetId = tweetId
			return 1, nil
		}}, users, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.ReactionEvent{TweetId: domain.RetweetPrefix + tweetId, OwnerId: owner, UserId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped tweet id %q, got %q", tweetId, capturedTweetId)
		}
	})

	t.Run("successful stream", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(stubReactionRepo{}, users, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return []byte("{}"), nil
			},
		})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.ReactionsCountResponse).Count != 1 {
			t.Fatalf("unexpected count: %v", resp)
		}
	})

	t.Run("answers an empty breakdown when the lookup fails", func(t *testing.T) {
		repo := stubReactionRepo{
			reactionsFn: func(tweetId string) (map[string]uint64, error) {
				return nil, errors.New("db error")
			},
		}
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(repo, users, stubModerationNotifier{}, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: owner}), conn)
		if err != nil {
			t.Fatalf("a failing breakdown must not fail the reaction: %v", err)
		}
		countResp := resp.(event.ReactionsCountResponse)
		if countResp.Reactions == nil || len(countResp.Reactions) != 0 {
			t.Fatalf("expected an empty map, got %#v", countResp.Reactions)
		}
	})

	t.Run("carries the reaction and echoes the breakdown", func(t *testing.T) {
		var (
			storedEmoji   string
			forwardederev event.ReactionEvent
		)
		repo := stubReactionRepo{
			reactFn: func(tweetId, userId, emoji string) (uint64, error) {
				storedEmoji = emoji
				return 3, nil
			},
			reactionsFn: func(tweetId string) (map[string]uint64, error) {
				return map[string]uint64{"🔥": 2, "❤️": 1}, nil
			},
		}
		users, conn := actorStream(t, owner, nil)
		h := StreamReactionHandler(repo, users, stubModerationNotifier{}, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				forwardederev = data.(event.ReactionEvent)
				return []byte("{}"), nil
			},
		})
		resp, err := h(marshal(t, event.ReactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner, Emoji: "🔥"}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if storedEmoji != "🔥" {
			t.Fatalf("expected the emoji to reach the repo, got %q", storedEmoji)
		}
		if forwardederev.Emoji != "🔥" {
			t.Fatalf("expected the emoji to be forwarded to the author's node, got %q", forwardederev.Emoji)
		}
		countResp := resp.(event.ReactionsCountResponse)
		if countResp.Count != 3 || countResp.Reactions["🔥"] != 2 || countResp.Reactions["❤️"] != 1 {
			t.Fatalf("unexpected response: %+v", countResp)
		}
	})
}

func TestStreamUnreactionHandler(t *testing.T) {
	owner := "owner-1"
	tweetOwner := "tweet-owner"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnreactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamUnreactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty tweet id", func(t *testing.T) {
		h := StreamUnreactionHandler(stubReactionRepo{}, stubReactionUserRepo{}, stubStreamer{})
		_, err := h(marshal(t, event.UnreactionEvent{OwnerId: owner, UserId: tweetOwner}), nil)
		if err == nil || err.Error() != "empty tweet id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("unreaction repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		users, conn := actorStream(t, owner, nil)
		h := StreamUnreactionHandler(stubReactionRepo{unreactFn: func(tweetId, userId string) (uint64, error) {
			return 0, repoErr
		}}, users, stubStreamer{})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("own tweet unreaction", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		h := StreamUnreactionHandler(stubReactionRepo{}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		resp, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.ReactionsCountResponse)
	})

	t.Run("someone else unreacted (exchange finished)", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		resp, err := StreamUnreactionHandler(stubReactionRepo{}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: "other-node"}})(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.ReactionsCountResponse)
	})

	t.Run("unreacted user not found", func(t *testing.T) {
		users, conn := actorStream(t, owner, func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		})
		h := StreamUnreactionHandler(stubReactionRepo{}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream node offline", func(t *testing.T) {
		users, conn := actorStream(t, owner, nil)
		h := StreamUnreactionHandler(stubReactionRepo{}, users, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, warpnet.ErrNodeIsOffline
			},
		})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		users, conn := actorStream(t, owner, nil)
		h := StreamUnreactionHandler(stubReactionRepo{}, users, stubStreamer{
			nodeInfo: warpnet.NodeInfo{OwnerId: owner},
			genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
				return nil, streamErr
			},
		})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: tweetId, OwnerId: owner, UserId: tweetOwner}), conn)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("strips retweet prefix", func(t *testing.T) {
		var capturedTweetId string
		users, conn := actorStream(t, owner, nil)
		h := StreamUnreactionHandler(stubReactionRepo{unreactFn: func(tweetId, userId string) (uint64, error) {
			capturedTweetId = tweetId
			return 0, nil
		}}, users, stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}})
		_, err := h(marshal(t, event.UnreactionEvent{TweetId: domain.RetweetPrefix + tweetId, OwnerId: owner, UserId: owner}), conn)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedTweetId != tweetId {
			t.Fatalf("expected stripped id %q, got %q", tweetId, capturedTweetId)
		}
	})
}

func TestStreamGetReactionsHandler(t *testing.T) {
	userId := "user-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetReactionsHandler(stubReactionRepo{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetReactionsHandler(stubReactionRepo{})
		_, err := h(marshal(t, event.GetReactionsEvent{}), nil)
		if err == nil || err.Error() != "reactions: empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamGetReactionsHandler(stubReactionRepo{reactedFn: func(userId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error) {
			return nil, "", repoErr
		}})
		_, err := h(marshal(t, event.GetReactionsEvent{UserId: userId}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error, got: %v", err)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		h := StreamGetReactionsHandler(stubReactionRepo{reactedFn: func(gotUserId string, limit *uint64, cursor *string) ([]domain.ReactedTweet, string, error) {
			if gotUserId != userId {
				t.Fatalf("unexpected user id: %q", gotUserId)
			}
			return []domain.ReactedTweet{
				{UserId: userId, TweetId: "tweet-1", OwnerUserId: "author-1"},
			}, "end", nil
		}})
		resp, err := h(marshal(t, event.GetReactionsEvent{UserId: userId}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		reactionsResp, ok := resp.(event.GetReactionsResponse)
		if !ok {
			t.Fatalf("unexpected response type: %T", resp)
		}
		if reactionsResp.Cursor != "end" || len(reactionsResp.Items) != 1 {
			t.Fatalf("unexpected response: %+v", reactionsResp)
		}
		if reactionsResp.Items[0].TweetId != "tweet-1" || reactionsResp.Items[0].OwnerUserId != "author-1" {
			t.Fatalf("unexpected item: %+v", reactionsResp.Items[0])
		}
	})
}

func TestNormalizeReaction(t *testing.T) {
	emoji, err := normalizeReaction("")
	if err != nil || emoji != defaultReaction {
		t.Fatalf("expected %q, got %q (%v)", defaultReaction, emoji, err)
	}

	emoji, err = normalizeReaction("🔥")
	if err != nil || emoji != "🔥" {
		t.Fatalf("expected the emoji back, got %q (%v)", emoji, err)
	}

	for _, bad := range []string{"🔥/💧", "a b", "\x00", "way too many emoji 🔥🔥🔥"} {
		if _, err := normalizeReaction(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}
