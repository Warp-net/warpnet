//nolint:all
package handler

import (
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

// These tests exercise the full notification write-then-read path using a real
// in-memory NotificationsRepo. They catch any regression where a handler stores
// a notification under a user id that StreamGetNotificationsHandler will not
// query for, which would silently hide notifications from their intended
// recipient.

func newNotificationsRepo(t *testing.T) *database.NotificationsRepo {
	t.Helper()
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatalf("local-store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	authRepo := database.NewAuthRepo(db, "test")
	if err := authRepo.Authenticate("test", "test"); err != nil {
		t.Fatalf("auth: %v", err)
	}
	return database.NewNotificationsRepo(db)
}

func listNotificationsFor(t *testing.T, repo *database.NotificationsRepo, recipient string) event.GetNotificationsResponse {
	t.Helper()
	h := StreamGetNotificationsHandler(repo, stubAuth{owner: domain.Owner{UserId: recipient}})
	resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
	if err != nil {
		t.Fatalf("get notifications: %v", err)
	}
	return resp.(event.GetNotificationsResponse)
}

func TestNotificationsDeliveredToRecipient_Reply(t *testing.T) {
	repo := newNotificationsRepo(t)

	tweetOwner := "alice"
	replier := "bob"
	rootId := "root-1"
	parentId := "parent-1"
	nodeID := warpnet.WarpPeerID("alice-node")

	h := StreamNewReplyHandler(
		stubReplyRepo{},
		stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
			return domain.User{Id: userId, NodeId: nodeID.String()}, nil
		}},
		repo,
		stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner, ID: nodeID}},
	)

	ev := event.NewReplyEvent{
		CreatedAt:    time.Now(),
		Id:           "reply-1",
		ParentId:     &parentId,
		ParentUserId: tweetOwner,
		RootId:       rootId,
		Text:         "a reply",
		UserId:       replier,
		Username:     "bob_user",
	}
	if _, err := h(marshal(t, ev), nil); err != nil {
		t.Fatalf("handler: %v", err)
	}

	r := listNotificationsFor(t, repo, tweetOwner)
	if len(r.Notifications) != 1 {
		t.Fatalf("expected 1 notification for %q, got %d", tweetOwner, len(r.Notifications))
	}
	if r.Notifications[0].Type != domain.NotificationReplyType {
		t.Fatalf("expected reply type, got: %v", r.Notifications[0].Type)
	}
	if r.UnreadCount != 1 {
		t.Fatalf("expected 1 unread, got %d", r.UnreadCount)
	}

	// Replier must not receive their own notification.
	if other := listNotificationsFor(t, repo, replier); len(other.Notifications) != 0 {
		t.Fatalf("replier should not receive the notification, got %d", len(other.Notifications))
	}
}

func TestNotificationsDeliveredToRecipient_Like(t *testing.T) {
	repo := newNotificationsRepo(t)

	tweetOwner := "alice"
	liker := "bob"

	h := StreamLikeHandler(
		stubLikeRepo{},
		stubLikeUserRepo{},
		repo,
		stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner}},
	)

	ev := event.LikeEvent{TweetId: "tweet-1", OwnerId: liker, UserId: tweetOwner}
	if _, err := h(marshal(t, ev), nil); err != nil {
		t.Fatalf("handler: %v", err)
	}

	r := listNotificationsFor(t, repo, tweetOwner)
	if len(r.Notifications) != 1 {
		t.Fatalf("expected 1 notification for %q, got %d", tweetOwner, len(r.Notifications))
	}
	if r.Notifications[0].Type != domain.NotificationLikeType {
		t.Fatalf("expected like type, got: %v", r.Notifications[0].Type)
	}

	if other := listNotificationsFor(t, repo, liker); len(other.Notifications) != 0 {
		t.Fatalf("liker should not receive the notification, got %d", len(other.Notifications))
	}
}

func TestNotificationsDeliveredToRecipient_Retweet(t *testing.T) {
	repo := newNotificationsRepo(t)

	tweetOwner := "alice"
	retweeter := "bob"

	h := StreamNewReTweetHandler(
		stubRetweetUserRepo{},
		stubReTweetRepo{},
		stubTimelineRepo{},
		repo,
		stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: tweetOwner}},
	)

	rt := retweeter
	tw := domain.Tweet{
		Id:          "tweet-1",
		UserId:      tweetOwner,
		Text:        "original",
		RetweetedBy: &rt,
		CreatedAt:   time.Now(),
	}
	if _, err := h(marshal(t, event.NewRetweetEvent(tw)), nil); err != nil {
		t.Fatalf("handler: %v", err)
	}

	r := listNotificationsFor(t, repo, tweetOwner)
	if len(r.Notifications) != 1 {
		t.Fatalf("expected 1 notification for %q, got %d", tweetOwner, len(r.Notifications))
	}
	if r.Notifications[0].Type != domain.NotificationRetweetType {
		t.Fatalf("expected retweet type, got: %v", r.Notifications[0].Type)
	}

	if other := listNotificationsFor(t, repo, retweeter); len(other.Notifications) != 0 {
		t.Fatalf("retweeter should not receive the notification, got %d", len(other.Notifications))
	}
}

func TestNotificationsDeliveredToRecipient_Follow(t *testing.T) {
	repo := newNotificationsRepo(t)

	followed := "alice"
	follower := "bob"

	h := StreamFollowHandler(
		stubFollowBroadcaster{},
		stubFollowRepo{},
		stubAuth{owner: domain.Owner{UserId: followed}},
		stubFollowUserRepo{},
		repo,
		stubFollowStreamer{},
	)

	ev := event.NewFollowEvent{FollowerId: follower, FollowingId: followed}
	if _, err := h(marshal(t, ev), nil); err != nil {
		t.Fatalf("handler: %v", err)
	}

	r := listNotificationsFor(t, repo, followed)
	if len(r.Notifications) != 1 {
		t.Fatalf("expected 1 notification for %q, got %d", followed, len(r.Notifications))
	}
	if r.Notifications[0].Type != domain.NotificationFollowType {
		t.Fatalf("expected follow type, got: %v", r.Notifications[0].Type)
	}

	if other := listNotificationsFor(t, repo, follower); len(other.Notifications) != 0 {
		t.Fatalf("follower should not receive the notification, got %d", len(other.Notifications))
	}
}

// Self-actions must not produce a notification. If any of these start
// producing one, the handler's guard logic has regressed.
func TestNotifications_NoSelfNotification(t *testing.T) {
	t.Run("self reply", func(t *testing.T) {
		repo := newNotificationsRepo(t)
		owner := "alice"
		parentId := "parent-1"
		nodeID := warpnet.WarpPeerID("alice-node")

		h := StreamNewReplyHandler(
			stubReplyRepo{},
			stubReplyUserRepo{getFn: func(userId string) (domain.User, error) {
				return domain.User{Id: userId, NodeId: nodeID.String()}, nil
			}},
			repo,
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner, ID: nodeID}},
		)

		ev := event.NewReplyEvent{
			CreatedAt:    time.Now(),
			Id:           "reply-1",
			ParentId:     &parentId,
			ParentUserId: owner,
			RootId:       "root-1",
			Text:         "self reply",
			UserId:       owner,
			Username:     "alice_user",
		}
		if _, err := h(marshal(t, ev), nil); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if r := listNotificationsFor(t, repo, owner); len(r.Notifications) != 0 {
			t.Fatalf("expected no self-notification, got %d", len(r.Notifications))
		}
	})

	t.Run("self like", func(t *testing.T) {
		repo := newNotificationsRepo(t)
		owner := "alice"
		h := StreamLikeHandler(
			stubLikeRepo{},
			stubLikeUserRepo{},
			repo,
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}},
		)
		ev := event.LikeEvent{TweetId: "tweet-1", OwnerId: owner, UserId: owner}
		if _, err := h(marshal(t, ev), nil); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if r := listNotificationsFor(t, repo, owner); len(r.Notifications) != 0 {
			t.Fatalf("expected no self-notification, got %d", len(r.Notifications))
		}
	})

	t.Run("self retweet", func(t *testing.T) {
		repo := newNotificationsRepo(t)
		owner := "alice"
		h := StreamNewReTweetHandler(
			stubRetweetUserRepo{},
			stubReTweetRepo{},
			stubTimelineRepo{},
			repo,
			stubStreamer{nodeInfo: warpnet.NodeInfo{OwnerId: owner}},
		)
		rt := owner
		tw := domain.Tweet{
			Id:          "tweet-1",
			UserId:      owner,
			Text:        "original",
			RetweetedBy: &rt,
			CreatedAt:   time.Now(),
		}
		if _, err := h(marshal(t, event.NewRetweetEvent(tw)), nil); err != nil {
			t.Fatalf("handler: %v", err)
		}
		if r := listNotificationsFor(t, repo, owner); len(r.Notifications) != 0 {
			t.Fatalf("expected no self-notification, got %d", len(r.Notifications))
		}
	})
}
