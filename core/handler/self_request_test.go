//nolint:all
package handler

// Tests in this file guard against a recurring class of bugs surfacing as
// "self request is not allowed" errors. The error originates from
// core/node.WarpNode.Stream when a handler asks the streamer to dial the
// owner's own node. Whenever a handler can recognize that the requested
// resource belongs to the owner, it must serve the answer locally and skip
// the outbound stream call entirely.
//
// Each subtest below installs a streamer that fails the test if its
// GenericStream is ever invoked, then drives the handler with an event that
// targets the owner.

import (
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

// failOnStream returns a GenericStream func that fails the test if the
// handler tries to dial any peer. It mimics the real ErrSelfRequest path so
// regressions (a handler accidentally dialing the owner's own node) become
// visible test failures rather than silent runtime errors.
func failOnStream(t *testing.T) func(string, stream.WarpRoute, any) ([]byte, error) {
	t.Helper()
	return func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
		t.Fatalf("owner self request: GenericStream must not be called (nodeId=%q path=%q)", nodeId, path)
		return nil, nil
	}
}

func TestOwnerSelfRequest_NoOutboundStream(t *testing.T) {
	const (
		owner   = "owner-1"
		tweetID = "tweet-1"
		rootID  = "root-1"
		chatID  = "chat-1"
	)
	// peer.ID.String() encodes the binary peer id, so handlers compare the
	// stored NodeId to NodeInfo.ID.String(). Using the encoded value here
	// keeps the comparison faithful to runtime behavior.
	ownerPeerID := warpnet.WarpPeerID("owner-node")
	ownerNodeID := ownerPeerID.String()
	ownerInfo := warpnet.NodeInfo{OwnerId: owner, ID: ownerPeerID}
	auth := stubAuth{owner: domain.Owner{UserId: owner, NodeId: ownerNodeID}}

	// Each of the user repos below returns a record whose NodeId points at
	// the owner's own node. A buggy handler would forward this to the
	// streamer and trigger ErrSelfRequest.
	ownerUserRepo := stubUserFetcher{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID, Network: warpnet.WarpnetName}, nil
	}}
	ownerFollowUserRepo := stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID}, nil
	}}
	ownerTweetUserRepo := stubTweetUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID}, nil
	}}
	ownerLikeUserRepo := stubLikeUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID}, nil
	}}
	ownerRetweetUserRepo := stubRetweetUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID}, nil
	}}
	ownerChatUserRepo := stubUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: ownerNodeID}, nil
	}}

	t.Run("StreamGetUserHandler - own profile", func(t *testing.T) {
		streamer := stubUserStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamGetUserHandler(stubUserTweetsCounter{}, stubUserFollowsCounter{}, ownerUserRepo, auth, streamer)
		if _, err := h(marshal(t, event.GetUserEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetUsersHandler - owner refresh skipped", func(t *testing.T) {
		streamer := stubUserStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamGetUsersHandler(ownerUserRepo, streamer)
		if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetTweetHandler - own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamGetTweetHandler(stubTweetRepo{}, auth, ownerTweetUserRepo, streamer)
		if _, err := h(marshal(t, event.GetTweetEvent{TweetId: tweetID, UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetTweetsHandler - owner refresh skipped", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		repo := stubTweetRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error) {
			return []domain.Tweet{{Id: tweetID, UserId: userId}}, "end", nil
		}}
		h := StreamGetTweetsHandler(repo, ownerTweetUserRepo, streamer)
		if _, err := h(marshal(t, event.GetAllTweetsEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetTweetStatsHandler - own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamGetTweetStatsHandler(stubTweetRepo{}, stubTweetLikeRepo{}, stubTweetRetweetRepo{}, stubRepliesCounter{}, ownerTweetUserRepo, streamer)
		if _, err := h(marshal(t, event.GetTweetStatsEvent{TweetId: tweetID, UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamFollowHandler - someone follows owner", func(t *testing.T) {
		streamer := stubFollowStreamer{genericStreamFn: failOnStream(t)}
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, auth, ownerFollowUserRepo, stubModerationNotifier{}, streamer)
		if _, err := h(marshal(t, event.NewFollowEvent{FollowerId: "stranger", FollowingId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamUnfollowHandler - someone unfollows owner", func(t *testing.T) {
		streamer := stubFollowStreamer{genericStreamFn: failOnStream(t)}
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, auth, ownerFollowUserRepo, streamer)
		if _, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: "stranger", FollowingId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetFollowersHandler - own followers", func(t *testing.T) {
		streamer := stubFollowStreamer{genericStreamFn: failOnStream(t)}
		h := StreamGetFollowersHandler(auth, ownerFollowUserRepo, stubFollowRepo{}, streamer)
		if _, err := h(marshal(t, event.GetFollowersEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetFollowingsHandler - own followings", func(t *testing.T) {
		streamer := stubFollowStreamer{genericStreamFn: failOnStream(t)}
		h := StreamGetFollowingsHandler(auth, ownerFollowUserRepo, stubFollowRepo{}, streamer)
		if _, err := h(marshal(t, event.GetFollowingsEvent{UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamCreateChatHandler - self chat", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		chatId := chatID
		h := StreamCreateChatHandler(stubChatRepo{}, ownerChatUserRepo, streamer)
		if _, err := h(marshal(t, event.NewChatEvent{ChatId: &chatId, OwnerId: owner, OtherUserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamNewMessageHandler - self message", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		repo := stubChatRepo{getChatFn: func(id string) (domain.Chat, error) {
			return domain.Chat{Id: id, OwnerId: owner, OtherUserId: owner}, nil
		}}
		h := StreamNewMessageHandler(repo, ownerChatUserRepo, streamer)
		// chatId must contain ":" to satisfy the parameter validation.
		if _, err := h(marshal(t, event.NewMessageEvent{ChatId: owner + ":" + owner, SenderId: owner, ReceiverId: owner, Text: "hi"}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamLikeHandler - own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamLikeHandler(stubLikeRepo{}, ownerLikeUserRepo, stubModerationNotifier{}, streamer)
		if _, err := h(marshal(t, event.LikeEvent{TweetId: tweetID, OwnerId: owner, UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamUnlikeHandler - own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamUnlikeHandler(stubLikeRepo{}, ownerLikeUserRepo, streamer)
		if _, err := h(marshal(t, event.UnlikeEvent{TweetId: tweetID, OwnerId: owner, UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamNewReTweetHandler - retweet of own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		retweetedBy := "stranger"
		ev := event.NewRetweetEvent{
			Id:          tweetID,
			UserId:      owner,
			RetweetedBy: &retweetedBy,
		}
		h := StreamNewReTweetHandler(ownerRetweetUserRepo, stubReTweetRepo{}, stubTimelineRepo{}, stubModerationNotifier{}, streamer)
		if _, err := h(marshal(t, ev), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamUnretweetHandler - unretweet of own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		repo := stubReTweetRepo{getFn: func(userID, tweetId string) (domain.Tweet, error) {
			return domain.Tweet{Id: tweetId, UserId: owner}, nil
		}}
		h := StreamUnretweetHandler(repo, ownerRetweetUserRepo, streamer)
		if _, err := h(marshal(t, event.UnretweetEvent{TweetId: tweetID, RetweeterId: "stranger"}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetReplyHandler - own reply", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamGetReplyHandler(stubReplyRepo{}, auth, ownerTweetUserRepo, streamer)
		if _, err := h(marshal(t, event.GetReplyEvent{ReplyId: "reply-1", RootId: rootID, UserId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamGetRepliesHandler - replies under own tweet", func(t *testing.T) {
		// No bridge node resolves and the streamer returns nothing, so the
		// request is served from the local store (see reply.go).
		h := StreamGetRepliesHandler(stubReplyRepo{}, stubReplyUserRepo{}, stubStreamer{})
		if _, err := h(marshal(t, event.GetAllRepliesEvent{RootId: rootID, ParentId: owner}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamNewReplyHandler - reply to own tweet", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		// The reply targets a parent tweet whose author lives on the owner's
		// node — the handler must serve it locally.
		userRepo := stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: ownerNodeID}, nil
		}}
		parentID := "parent-1"
		ev := event.NewReplyEvent{
			Id:           "reply-1",
			ParentId:     &parentID,
			ParentUserId: owner,
			RootId:       rootID,
			Text:         "hello",
			UserId:       "stranger",
			Username:     "stranger",
		}
		h := StreamNewReplyHandler(stubReplyRepo{}, userRepo, stubModerationNotifier{}, streamer)
		if _, err := h(marshal(t, ev), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("StreamViewHandler - author is owner", func(t *testing.T) {
		streamer := stubStreamer{
			nodeInfo:        ownerInfo,
			genericStreamFn: failOnStream(t),
		}
		h := StreamViewHandler(stubViewRepo{}, ownerLikeUserRepo, streamer)
		if _, err := h(marshal(t, event.ViewEvent{TweetId: tweetID, UserId: owner, ViewerId: "stranger"}), nil); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})
}

// Fix #1 regression: refreshUsers used to persist every user from a remote
// response, including records that point back at this node. Once such a
// record landed in the local repo, every later fetch of that user dialed
// self and surfaced node.ErrSelfRequest. The handler must drop them.
func TestStreamGetUsersHandler_DropsSelfPointingRecords(t *testing.T) {
	owner := "owner-1"
	ownerPeerID := warpnet.WarpPeerID("owner-node")
	ownerNodeID := ownerPeerID.String()

	persisted := map[string]domain.User{}
	repo := stubUserFetcher{
		listFn: func(limit *uint64, cursor *string) ([]domain.User, string, error) {
			// Force the refresh path by returning no local users on the
			// first call; on subsequent calls return whatever has been
			// persisted so the handler has something to respond with.
			if len(persisted) == 0 {
				return nil, "", nil
			}
			out := make([]domain.User, 0, len(persisted))
			for _, u := range persisted {
				out = append(out, u)
			}
			return out, "end", nil
		},
		getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: "remote-node"}, nil
		},
		createFn: func(user domain.User) (domain.User, error) {
			persisted[user.Id] = user
			return user, nil
		},
		updateFn: func(userId string, user domain.User) (domain.User, error) {
			persisted[userId] = user
			return user, nil
		},
	}

	remoteResp := event.UsersResponse{
		Users: []domain.User{
			{Id: "valid-other", NodeId: "remote-node-2", Network: warpnet.WarpnetName},
			{Id: "self-by-node", NodeId: ownerNodeID, Network: warpnet.WarpnetName},
			{Id: owner, NodeId: "remote-node-3", Network: warpnet.WarpnetName},
		},
	}
	respBytes := marshal(t, remoteResp)

	streamer := stubUserStreamer{
		nodeInfo: warpnet.NodeInfo{OwnerId: owner, ID: ownerPeerID},
		genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return respBytes, nil
		},
	}

	h := StreamGetUsersHandler(repo, streamer)
	if _, err := h(marshal(t, event.GetAllUsersEvent{UserId: "requester"}), nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if _, ok := persisted["valid-other"]; !ok {
		t.Fatalf("expected legitimate remote user to be persisted: %v", persisted)
	}
	if _, ok := persisted["self-by-node"]; ok {
		t.Fatalf("record with self NodeId must be discarded, got: %v", persisted["self-by-node"])
	}
	if _, ok := persisted[owner]; ok {
		t.Fatalf("record with owner Id must be discarded, got: %v", persisted[owner])
	}
}

// Fix #2 regression: StreamNewReplyHandler used to detect "reply to my own
// tweet" only via parentUser.NodeId == streamer.NodeInfo().ID.String(). When
// the stored NodeId drifted from the runtime peer.ID encoding, that check
// silently returned false and the handler dialed self. The check must also
// match by owner id.
func TestStreamNewReplyHandler_OwnTweet_NodeIdFormatDrift(t *testing.T) {
	owner := "owner-1"
	ownerPeerID := warpnet.WarpPeerID("owner-node")
	rootID := "root-1"
	parentID := "parent-1"

	streamer := stubStreamer{
		nodeInfo:        warpnet.NodeInfo{OwnerId: owner, ID: ownerPeerID},
		genericStreamFn: failOnStream(t),
	}

	// parentUser.NodeId here intentionally diverges from
	// ownerPeerID.String() to simulate a stored record whose NodeId was
	// captured in a different encoding. Only the owner-id branch can save us.
	userRepo := stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: "stale-encoded-form"}, nil
	}}

	ev := event.NewReplyEvent{
		Id:           "reply-1",
		ParentId:     &parentID,
		ParentUserId: owner,
		RootId:       rootID,
		Text:         "reply body",
		UserId:       "stranger",
		Username:     "stranger",
	}

	h := StreamNewReplyHandler(stubReplyRepo{}, userRepo, stubModerationNotifier{}, streamer)
	if _, err := h(marshal(t, ev), nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
