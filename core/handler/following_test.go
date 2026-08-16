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
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFollowRepo struct {
	followFn        func(from, to string) error
	unfollowFn      func(from, to string) error
	getFollowersFn  func(userId string, limit *uint64, cursor *string) ([]string, string, error)
	getFollowingsFn func(userId string, limit *uint64, cursor *string) ([]string, string, error)
	isFollowingFn   func(ownerId, otherUserId string) bool
	isFollowerFn    func(ownerId, otherUserId string) bool

	addFollowRequestFn    func(targetUserId, followerId string) error
	removeFollowRequestFn func(targetUserId, followerId string) error
	listFollowRequestsFn  func(targetUserId string, limit *uint64, cursor *string) ([]string, string, error)
}

func (s stubFollowRepo) Follow(from, to string) error {
	if s.followFn != nil {
		return s.followFn(from, to)
	}
	return nil
}
func (s stubFollowRepo) Unfollow(from, to string) error {
	if s.unfollowFn != nil {
		return s.unfollowFn(from, to)
	}
	return nil
}
func (s stubFollowRepo) GetFollowers(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.getFollowersFn != nil {
		return s.getFollowersFn(userId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubFollowRepo) GetFollowings(userId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.getFollowingsFn != nil {
		return s.getFollowingsFn(userId, limit, cursor)
	}
	return nil, "", nil
}
func (s stubFollowRepo) IsFollowing(ownerId, otherUserId string) bool {
	if s.isFollowingFn != nil {
		return s.isFollowingFn(ownerId, otherUserId)
	}
	return false
}
func (s stubFollowRepo) AddFollowRequest(targetUserId, followerId string) error {
	if s.addFollowRequestFn != nil {
		return s.addFollowRequestFn(targetUserId, followerId)
	}
	return nil
}

func (s stubFollowRepo) RemoveFollowRequest(targetUserId, followerId string) error {
	if s.removeFollowRequestFn != nil {
		return s.removeFollowRequestFn(targetUserId, followerId)
	}
	return nil
}

func (s stubFollowRepo) ListFollowRequests(targetUserId string, limit *uint64, cursor *string) ([]string, string, error) {
	if s.listFollowRequestsFn != nil {
		return s.listFollowRequestsFn(targetUserId, limit, cursor)
	}
	return nil, "end", nil
}

func (s stubFollowRepo) IsFollower(ownerId, otherUserId string) bool {
	if s.isFollowerFn != nil {
		return s.isFollowerFn(ownerId, otherUserId)
	}
	return false
}

type stubFollowBroadcaster struct {
	subscribeFn   func(userId string) error
	unsubscribeFn func(userId string) error
}

func (s stubFollowBroadcaster) SubscribeUserUpdate(userId string) error {
	if s.subscribeFn != nil {
		return s.subscribeFn(userId)
	}
	return nil
}
func (s stubFollowBroadcaster) UnsubscribeUserUpdate(userId string) error {
	if s.unsubscribeFn != nil {
		return s.unsubscribeFn(userId)
	}
	return nil
}

type stubFollowUserRepo struct {
	getFn    func(userId string) (domain.User, error)
	listFn   func(limit *uint64, cursor *string) ([]domain.User, string, error)
	createFn func(user domain.User) (domain.User, error)
}

func (s stubFollowUserRepo) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId, NodeId: "node-2"}, nil
}
func (s stubFollowUserRepo) List(limit *uint64, cursor *string) ([]domain.User, string, error) {
	if s.listFn != nil {
		return s.listFn(limit, cursor)
	}
	return nil, "", nil
}
func (s stubFollowUserRepo) Create(user domain.User) (domain.User, error) {
	if s.createFn != nil {
		return s.createFn(user)
	}
	return user, nil
}

type stubFollowStreamer struct {
	genericStreamFn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

func (s stubFollowStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.genericStreamFn != nil {
		return s.genericStreamFn(nodeId, path, data)
	}
	return nil, nil
}

// actorNodeStream returns a fresh node id plus a stream arriving from that
// node: POST handlers accept an event only when the claimed actor's user
// record carries the delivering peer's id.
func actorNodeStream(t *testing.T) (warpnet.WarpPeerID, warpnet.WarpStream) {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate actor key: %v", err)
	}
	nodeId, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive actor node id: %v", err)
	}
	_, server := stream.NewLoopbackStream(nodeId, nodeId, "/test/route/0.0.0")
	return nodeId, server
}

func TestStreamFollowHandler(t *testing.T) {
	owner := "owner-1"
	following := "following-1"

	nodeId, senderStream := actorNodeStream(t)
	senderNode := nodeId.String()
	// Follower profiles pinned to the delivering node.
	senderUsers := stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: senderNode}, nil
	}}
	// Owner-initiated follows must arrive from the owner's own node.
	ownerAuth := stubAuth{owner: domain.Owner{UserId: owner, NodeId: senderNode}}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty follower id", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewFollowEvent{FollowingId: following}), nil)
		if err == nil || err.Error() != "empty follower or following id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty following id", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner}), nil)
		if err == nil || err.Error() != "empty follower or following id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("self follow returns accepted", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("someone follows me", func(t *testing.T) {
		var capturedFrom, capturedTo string
		notified := false
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{followFn: func(from, to string) error {
			capturedFrom = from
			capturedTo = to
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, senderUsers, stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationFollowType {
				t.Fatalf("expected follow type, got: %v", not.Type)
			}
			if not.RecepientId != owner {
				t.Fatalf("expected notification for followed user, got: %v", not.RecepientId)
			}
			return nil
		}}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: following, FollowingId: owner}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if capturedFrom != following || capturedTo != owner {
			t.Fatalf("expected Follow(%q, %q), got Follow(%q, %q)", following, owner, capturedFrom, capturedTo)
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("someone follows me - already followed", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{followFn: func(from, to string) error {
			return database.ErrAlreadyFollowed
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, senderUsers, stubModerationNotifier{}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: following, FollowingId: owner}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("someone follows me - repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{followFn: func(from, to string) error {
			return repoErr
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, senderUsers, stubModerationNotifier{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: following, FollowingId: owner}), senderStream)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("I follow someone - user not found", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubModerationNotifier{}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("I follow someone - user repo error", func(t *testing.T) {
		repoErr := errors.New("user fetch error")
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, repoErr
		}}, stubModerationNotifier{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected user repo error: %v", err)
		}
	})

	t.Run("I follow someone - node offline", func(t *testing.T) {
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if !errors.Is(err, warpnet.ErrUserIsOffline) {
			t.Fatalf("expected user offline error: %v", err)
		}
	})

	t.Run("I follow someone - stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, streamErr
		}})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("I follow someone - already followed locally", func(t *testing.T) {
		acceptedResp, _ := json.Marshal(event.Accepted)
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{followFn: func(from, to string) error {
			return database.ErrAlreadyFollowed
		}}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return acceptedResp, nil
		}})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("I follow someone - success", func(t *testing.T) {
		subscribed := false
		acceptedResp, _ := json.Marshal(event.Accepted)
		h := StreamFollowHandler(stubFollowBroadcaster{subscribeFn: func(userId string) error {
			subscribed = true
			return nil
		}}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return acceptedResp, nil
		}})
		resp, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !subscribed {
			t.Fatal("expected broadcaster subscribe to be called")
		}
	})

	t.Run("I follow someone - subscribe error", func(t *testing.T) {
		subErr := errors.New("subscribe failed")
		acceptedResp, _ := json.Marshal(event.Accepted)
		h := StreamFollowHandler(stubFollowBroadcaster{subscribeFn: func(userId string) error {
			return subErr
		}}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return acceptedResp, nil
		}})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if !errors.Is(err, subErr) {
			t.Fatalf("expected subscribe error: %v", err)
		}
	})

	t.Run("I follow someone - remote error response", func(t *testing.T) {
		errResp, _ := json.Marshal(event.ResponseError{Code: 500, Message: "remote error"})
		h := StreamFollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubModerationNotifier{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return errResp, nil
		}})
		_, err := h(marshal(t, event.NewFollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if err == nil {
			t.Fatal("expected error from remote error response")
		}
	})
}

// TestStreamFollowHandlerFetchesUnknownFollower covers the rule that a follower
// we don't know yet is resolved from the node that delivered the follow before
// the follow is stored — on any network. Without it the follower stays an id, so
// the notification reads as that raw id and the followers list has an empty row.
func TestStreamFollowHandlerFetchesUnknownFollower(t *testing.T) {
	owner := "owner-1"
	unknown := "ap:aHR0cHM6Ly9tYXN0b2Rvbi5zb2NpYWwvdXNlcnMvd2FycG5ldA"
	senderPeer, _ := peer.Decode("QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6")
	senderStream := stubPairStream{conn: stubPairConn{remotePeerID: senderPeer}}

	userRepo := func(created *domain.User) stubFollowUserRepo {
		return stubFollowUserRepo{
			getFn: func(userId string) (domain.User, error) {
				if userId == unknown {
					return domain.User{}, database.ErrUserNotFound
				}
				return domain.User{Id: userId}, nil
			},
			createFn: func(u domain.User) (domain.User, error) { *created = u; return u, nil },
		}
	}

	t.Run("resolves from the sender, then stores the follow", func(t *testing.T) {
		var created domain.User
		var askedNode, notified string
		streamer := stubFollowStreamer{
			genericStreamFn: func(nodeId string, path stream.WarpRoute, _ any) ([]byte, error) {
				askedNode = nodeId
				if path != event.PUBLIC_GET_USER {
					t.Fatalf("path = %q, want %q", path, event.PUBLIC_GET_USER)
				}
				return json.Marshal(domain.User{Id: unknown, Username: "Warpnet", NodeId: senderPeer.String()})
			},
		}
		followed := false
		h := StreamFollowHandler(
			stubFollowBroadcaster{},
			stubFollowRepo{followFn: func(string, string) error { followed = true; return nil }},
			stubAuth{owner: domain.Owner{UserId: owner}},
			userRepo(&created),
			stubModerationNotifier{addFn: func(not domain.Notification) error { notified = not.Text; return nil }},
			streamer,
		)
		if _, err := h(marshal(t, event.NewFollowEvent{FollowerId: unknown, FollowingId: owner}), senderStream); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if askedNode != senderPeer.String() {
			t.Fatalf("asked %q, want the delivering node %q", askedNode, senderPeer)
		}
		if created.Username != "Warpnet" {
			t.Fatalf("follower not stored: %+v", created)
		}
		if !followed {
			t.Fatal("follow was not stored")
		}
		if notified != "Warpnet started following you" {
			t.Fatalf("notification = %q", notified)
		}
	})

	t.Run("a follower already in the db is not requested", func(t *testing.T) {
		followed := false
		h := StreamFollowHandler(
			stubFollowBroadcaster{},
			stubFollowRepo{followFn: func(string, string) error { followed = true; return nil }},
			stubAuth{owner: domain.Owner{UserId: owner}},
			stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: userId, Username: "Vadim", NodeId: senderPeer.String()}, nil
			}},
			stubModerationNotifier{},
			stubFollowStreamer{genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				t.Fatal("a known follower must not be requested from the network")
				return nil, nil
			}},
		)
		if _, err := h(marshal(t, event.NewFollowEvent{FollowerId: "known-1", FollowingId: owner}), senderStream); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !followed {
			t.Fatal("follow was not stored")
		}
	})

	t.Run("unresolvable follower is not followed", func(t *testing.T) {
		var created domain.User
		streamer := stubFollowStreamer{
			genericStreamFn: func(string, stream.WarpRoute, any) ([]byte, error) {
				return nil, errors.New("node unreachable")
			},
		}
		h := StreamFollowHandler(
			stubFollowBroadcaster{},
			stubFollowRepo{followFn: func(string, string) error {
				t.Fatal("follow must not be stored for an unresolved follower")
				return nil
			}},
			stubAuth{owner: domain.Owner{UserId: owner}},
			userRepo(&created),
			stubModerationNotifier{},
			streamer,
		)
		if _, err := h(marshal(t, event.NewFollowEvent{FollowerId: unknown, FollowingId: owner}), senderStream); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStreamUnfollowHandler(t *testing.T) {
	owner := "owner-1"
	following := "following-1"

	nodeId, senderStream := actorNodeStream(t)
	senderNode := nodeId.String()
	// Unfollower profiles pinned to the delivering node.
	senderUsers := stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
		return domain.User{Id: userId, NodeId: senderNode}, nil
	}}
	// Owner-initiated unfollows must arrive from the owner's own node.
	ownerAuth := stubAuth{owner: domain.Owner{UserId: owner, NodeId: senderNode}}

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty ids", func(t *testing.T) {
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner}), nil)
		if err == nil || err.Error() != "empty follower or following id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("self unfollow returns accepted", func(t *testing.T) {
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner, FollowingId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("someone unfollows me - correct arg order", func(t *testing.T) {
		var capturedFrom, capturedTo string
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{unfollowFn: func(from, to string) error {
			capturedFrom = from
			capturedTo = to
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, senderUsers, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: following, FollowingId: owner}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		// Verify the correct argument order: follower unfollows me
		if capturedFrom != following || capturedTo != owner {
			t.Fatalf("expected Unfollow(%q, %q), got Unfollow(%q, %q)", following, owner, capturedFrom, capturedTo)
		}
	})

	t.Run("someone unfollows me - repo error", func(t *testing.T) {
		repoErr := errors.New("db error")
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{unfollowFn: func(from, to string) error {
			return repoErr
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, senderUsers, stubFollowStreamer{})
		_, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: following, FollowingId: owner}), senderStream)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("I unfollow someone - user not found", func(t *testing.T) {
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
	})

	t.Run("I unfollow someone - node offline", func(t *testing.T) {
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}})
		_, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if !errors.Is(err, warpnet.ErrUserIsOffline) {
			t.Fatalf("expected user offline error: %v", err)
		}
	})

	t.Run("I unfollow someone - stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamUnfollowHandler(stubFollowBroadcaster{}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, streamErr
		}})
		_, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("I unfollow someone - success", func(t *testing.T) {
		unsubscribed := false
		acceptedResp, _ := json.Marshal(event.Accepted)
		h := StreamUnfollowHandler(stubFollowBroadcaster{unsubscribeFn: func(userId string) error {
			unsubscribed = true
			return nil
		}}, stubFollowRepo{}, ownerAuth, stubFollowUserRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return acceptedResp, nil
		}})
		resp, err := h(marshal(t, event.NewUnfollowEvent{FollowerId: owner, FollowingId: following}), senderStream)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !unsubscribed {
			t.Fatal("expected broadcaster unsubscribe to be called")
		}
	})
}

func TestStreamIsFollowingHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamIsFollowingHandler(stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamIsFollowingHandler(stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h(marshal(t, event.GetIsFollowingEvent{}), nil)
		if err == nil || err.Error() != "empty following id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("is following true", func(t *testing.T) {
		h := StreamIsFollowingHandler(stubFollowRepo{isFollowingFn: func(ownerId, otherUserId string) bool {
			return true
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetIsFollowingEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !resp.(event.IsFollowingResponse).IsFollowing {
			t.Fatal("expected is following true")
		}
	})

	t.Run("is following false", func(t *testing.T) {
		h := StreamIsFollowingHandler(stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetIsFollowingEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp.(event.IsFollowingResponse).IsFollowing {
			t.Fatal("expected is following false")
		}
	})
}

func TestStreamIsFollowerHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamIsFollowerHandler(stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamIsFollowerHandler(stubFollowRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h(marshal(t, event.GetIsFollowingEvent{}), nil)
		if err == nil || err.Error() != "empty follower id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("is follower true", func(t *testing.T) {
		h := StreamIsFollowerHandler(stubFollowRepo{isFollowerFn: func(ownerId, otherUserId string) bool {
			return true
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetIsFollowingEvent{UserId: "other-1"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !resp.(event.IsFollowerResponse).IsFollower {
			t.Fatal("expected is follower true")
		}
	})
}

func TestStreamGetFollowersHandler(t *testing.T) {
	owner := "owner-1"
	other := "other-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.GetFollowersEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own followers", func(t *testing.T) {
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{getFollowersFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
			return []string{"f1", "f2"}, "end", nil
		}}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.GetFollowersEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.FollowersResponse)
		if len(r.Followers) != 2 {
			t.Fatalf("expected 2 followers, got %d", len(r.Followers))
		}
	})

	t.Run("own followers - repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{getFollowersFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
			return nil, "", repoErr
		}}, stubFollowStreamer{})
		_, err := h(marshal(t, event.GetFollowersEvent{UserId: owner}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("other user followers - user not found fallback", func(t *testing.T) {
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{}, database.ErrUserNotFound
		}}, stubFollowRepo{getFollowersFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
			return []string{"cached-f1"}, "end", nil
		}}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.GetFollowersEvent{UserId: other}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.FollowersResponse)
		if len(r.Followers) != 1 || r.Followers[0] != "cached-f1" {
			t.Fatalf("expected cached followers fallback: %v", r)
		}
	})

	t.Run("other user followers - node offline fallback", func(t *testing.T) {
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}})
		resp, err := h(marshal(t, event.GetFollowersEvent{UserId: other}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.FollowersResponse)
	})

	t.Run("other user followers - stream error", func(t *testing.T) {
		streamErr := errors.New("stream broken")
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, streamErr
		}})
		_, err := h(marshal(t, event.GetFollowersEvent{UserId: other}), nil)
		if !errors.Is(err, streamErr) {
			t.Fatalf("expected stream error: %v", err)
		}
	})

	t.Run("other user followers - remote success", func(t *testing.T) {
		remoteResp, _ := json.Marshal(event.FollowersResponse{
			Cursor:      "end",
			FollowingId: other,
			Followers:   []string{"remote-f1", "remote-f2"},
		})
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return remoteResp, nil
		}})
		resp, err := h(marshal(t, event.GetFollowersEvent{UserId: other}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.FollowersResponse)
		if len(r.Followers) != 2 {
			t.Fatalf("expected 2 remote followers, got %d", len(r.Followers))
		}
	})

	t.Run("other user followers - remote error response", func(t *testing.T) {
		errResp, _ := json.Marshal(event.ResponseError{Code: 500, Message: "remote error"})
		h := StreamGetFollowersHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return errResp, nil
		}})
		_, err := h(marshal(t, event.GetFollowersEvent{UserId: other}), nil)
		if err == nil {
			t.Fatal("expected error from remote error response")
		}
	})
}

func TestStreamGetFollowingsHandler(t *testing.T) {
	owner := "owner-1"
	other := "other-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetFollowingsHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty user id", func(t *testing.T) {
		h := StreamGetFollowingsHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{})
		_, err := h(marshal(t, event.GetFollowingsEvent{}), nil)
		if err == nil || err.Error() != "empty user id" {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("own followings", func(t *testing.T) {
		h := StreamGetFollowingsHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{getFollowingsFn: func(userId string, limit *uint64, cursor *string) ([]string, string, error) {
			return []string{"f1"}, "end", nil
		}}, stubFollowStreamer{})
		resp, err := h(marshal(t, event.GetFollowingsEvent{UserId: owner}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.FollowingsResponse)
		if len(r.Followings) != 1 {
			t.Fatalf("expected 1 following, got %d", len(r.Followings))
		}
	})

	t.Run("other user followings - remote success", func(t *testing.T) {
		remoteResp, _ := json.Marshal(event.FollowingsResponse{
			Cursor:     "end",
			FollowerId: other,
			Followings: []string{"remote-f1"},
		})
		h := StreamGetFollowingsHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return remoteResp, nil
		}})
		resp, err := h(marshal(t, event.GetFollowingsEvent{UserId: other}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.FollowingsResponse)
		if len(r.Followings) != 1 {
			t.Fatalf("expected 1 remote following, got %d", len(r.Followings))
		}
	})

	t.Run("other user followings - node offline fallback", func(t *testing.T) {
		h := StreamGetFollowingsHandler(stubAuth{owner: domain.Owner{UserId: owner}}, stubFollowUserRepo{}, stubFollowRepo{}, stubFollowStreamer{genericStreamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			return nil, warpnet.ErrNodeIsOffline
		}})
		resp, err := h(marshal(t, event.GetFollowingsEvent{UserId: other}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_ = resp.(event.FollowingsResponse)
	})
}

func TestStreamGetFollowRequestsHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("empty user id", func(t *testing.T) {
		_, err := StreamGetFollowRequestsHandler(stubFollowRepo{})(
			mustJSON(t, event.GetFollowRequestsEvent{}), nil)
		assert.Error(t, err, "pending requests are private to the account owner")
	})

	t.Run("storage failure surfaces", func(t *testing.T) {
		repo := stubFollowRepo{listFollowRequestsFn: func(string, *uint64, *string) ([]string, string, error) {
			return nil, "", errors.New("db down")
		}}
		_, err := StreamGetFollowRequestsHandler(repo)(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		assert.Error(t, err)
	})

	t.Run("requests and cursor pass through", func(t *testing.T) {
		repo := stubFollowRepo{listFollowRequestsFn: func(string, *uint64, *string) ([]string, string, error) {
			return []string{"pending-1", "pending-2"}, "cur", nil
		}}
		out, err := StreamGetFollowRequestsHandler(repo)(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		require.NoError(t, err)

		resp := out.(event.GetFollowRequestsResponse)
		assert.Equal(t, []domain.ID{"pending-1", "pending-2"}, resp.FollowerIds)
		assert.Equal(t, "cur", resp.Cursor)
	})

	t.Run("no pending requests yields an empty slice", func(t *testing.T) {
		out, err := StreamGetFollowRequestsHandler(stubFollowRepo{})(
			mustJSON(t, event.GetFollowRequestsEvent{UserId: "u1"}), nil)
		require.NoError(t, err)
		assert.NotNil(t, out.(event.GetFollowRequestsResponse).FollowerIds)
	})
}

func TestStreamAuthorizeFollowRequestHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("missing identifiers are rejected", func(t *testing.T) {
		_, err := StreamAuthorizeFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		assert.Error(t, err)

		_, err = StreamAuthorizeFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		assert.Error(t, err)
	})

	t.Run("approval creates the follow in the right direction", func(t *testing.T) {
		var followFrom, followTo string
		var removedTarget, removedFollower string

		repo := stubFollowRepo{
			followFn: func(from, to string) error {
				followFrom, followTo = from, to
				return nil
			},
			removeFollowRequestFn: func(target, follower string) error {
				removedTarget, removedFollower = target, follower
				return nil
			},
		}

		out, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.Accepted, out)

		assert.Equal(t, "applicant", followFrom, "the applicant follows the owner")
		assert.Equal(t, "owner", followTo)
		assert.Equal(t, "owner", removedTarget, "the pending row is keyed by the owner")
		assert.Equal(t, "applicant", removedFollower)
	})

	t.Run("failed follow leaves the request pending", func(t *testing.T) {
		removeCalled := false
		repo := stubFollowRepo{
			followFn: func(string, string) error { return errors.New("storage full") },
			removeFollowRequestFn: func(string, string) error {
				removeCalled = true
				return nil
			},
		}

		_, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
		assert.False(t, removeCalled, "the pending request must survive a failed approval")
	})

	t.Run("failed cleanup surfaces", func(t *testing.T) {
		repo := stubFollowRepo{
			removeFollowRequestFn: func(string, string) error { return errors.New("db down") },
		}
		_, err := StreamAuthorizeFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
	})
}

func TestStreamRejectFollowRequestHandler(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowRepo{})([]byte("{"), nil)
		assert.Error(t, err)
	})

	t.Run("missing identifiers are rejected", func(t *testing.T) {
		_, err := StreamRejectFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{FollowerId: "f"}), nil)
		assert.Error(t, err)

		_, err = StreamRejectFollowRequestHandler(stubFollowRepo{})(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "u"}), nil)
		assert.Error(t, err)
	})

	t.Run("rejection removes the request and follows nobody", func(t *testing.T) {
		followCalled := false
		var removedTarget, removedFollower string

		repo := stubFollowRepo{
			followFn: func(string, string) error {
				followCalled = true
				return nil
			},
			removeFollowRequestFn: func(target, follower string) error {
				removedTarget, removedFollower = target, follower
				return nil
			},
		}

		out, err := StreamRejectFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		require.NoError(t, err)
		assert.Equal(t, event.Accepted, out)
		assert.False(t, followCalled, "a rejected applicant must not end up following the owner")
		assert.Equal(t, "owner", removedTarget)
		assert.Equal(t, "applicant", removedFollower)
	})

	t.Run("storage failure surfaces", func(t *testing.T) {
		repo := stubFollowRepo{
			removeFollowRequestFn: func(string, string) error { return errors.New("db down") },
		}
		_, err := StreamRejectFollowRequestHandler(repo)(
			mustJSON(t, event.FollowRequestActionEvent{UserId: "owner", FollowerId: "applicant"}), nil)
		assert.Error(t, err)
	})
}
