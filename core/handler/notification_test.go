//nolint:all
package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubNotificationRepo struct {
	listFn        func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error)
	listSinceFn   func(userId, since string, limit *uint64) ([]domain.Notification, string, error)
	getFn         func(userId, notificationId string) (domain.Notification, error)
	unreadCountFn func(userId string) (uint64, error)
}

func (s stubNotificationRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
	if s.listFn != nil {
		return s.listFn(userId, limit, cursor)
	}
	return nil, "", nil
}

func (s stubNotificationRepo) ListSince(userId, since string, limit *uint64) ([]domain.Notification, string, error) {
	if s.listSinceFn != nil {
		return s.listSinceFn(userId, since, limit)
	}
	return nil, "", nil
}

func (s stubNotificationRepo) Get(userId, notificationId string) (domain.Notification, error) {
	if s.getFn != nil {
		return s.getFn(userId, notificationId)
	}
	return domain.Notification{}, nil
}

func (s stubNotificationRepo) UnreadCount(userId string) (uint64, error) {
	if s.unreadCountFn != nil {
		return s.unreadCountFn(userId)
	}
	return 0, nil
}

func TestStreamGetNotificationsHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetNotificationsHandler(stubNotificationRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty notifications", func(t *testing.T) {
		h := StreamGetNotificationsHandler(stubNotificationRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 0 {
			t.Fatalf("expected 0 unread, got %d", r.UnreadCount)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetNotificationsHandler(stubNotificationRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
			return nil, "", repoErr
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("counts unread correctly and sorts unread first", func(t *testing.T) {
		now := time.Now()
		nots := []domain.Notification{
			{Id: "1", Type: domain.NotificationLikeType, IsRead: true, UserId: owner, CreatedAt: now.Add(-3 * time.Second)},
			{Id: "2", Type: domain.NotificationReplyType, IsRead: false, UserId: owner, CreatedAt: now.Add(-2 * time.Second)},
			{Id: "3", Type: domain.NotificationFollowType, IsRead: false, UserId: owner, CreatedAt: now.Add(-1 * time.Second)},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				return nots, "end", nil
			},
			unreadCountFn: func(userId string) (uint64, error) {
				return 2, nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 2 {
			t.Fatalf("expected 2 unread, got %d", r.UnreadCount)
		}
		if len(r.Notifications) != 3 {
			t.Fatalf("expected 3 notifications, got %d", len(r.Notifications))
		}
		// unread notifications should come first
		if r.Notifications[0].IsRead {
			t.Fatal("expected first notification to be unread")
		}
		if r.Notifications[1].IsRead {
			t.Fatal("expected second notification to be unread")
		}
		if !r.Notifications[2].IsRead {
			t.Fatal("expected third notification to be read")
		}
		// within unread group, newer should come first
		if r.Notifications[0].Id != "3" {
			t.Fatalf("expected newest unread first, got id=%s", r.Notifications[0].Id)
		}
		if r.Cursor != "end" {
			t.Fatalf("expected cursor 'end', got %q", r.Cursor)
		}
	})

	t.Run("all unread", func(t *testing.T) {
		nots := []domain.Notification{
			{Id: "1", Type: domain.NotificationLikeType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
			{Id: "2", Type: domain.NotificationReplyType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				return nots, "end", nil
			},
			unreadCountFn: func(userId string) (uint64, error) { return 2, nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 2 {
			t.Fatalf("expected 2 unread, got %d", r.UnreadCount)
		}
	})

	t.Run("all read", func(t *testing.T) {
		nots := []domain.Notification{
			{Id: "1", Type: domain.NotificationLikeType, IsRead: true, UserId: owner, CreatedAt: time.Now()},
			{Id: "2", Type: domain.NotificationReplyType, IsRead: true, UserId: owner, CreatedAt: time.Now()},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				return nots, "end", nil
			},
			unreadCountFn: func(userId string) (uint64, error) { return 0, nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 0 {
			t.Fatalf("expected 0 unread, got %d", r.UnreadCount)
		}
	})

	t.Run("unread count is global, not page-local", func(t *testing.T) {
		// One unread in the current page but 17 unread overall (older
		// pages or pages the cursor hasn't reached): the response must
		// carry 17, not 1. This is the bug behind the flickering
		// "N unread" badge.
		page := []domain.Notification{
			{Id: "1", Type: domain.NotificationLikeType, IsRead: true, UserId: owner, CreatedAt: time.Now()},
			{Id: "2", Type: domain.NotificationReplyType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				return page, "next", nil
			},
			unreadCountFn: func(userId string) (uint64, error) { return 17, nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 17 {
			t.Fatalf("expected global unread count 17, got %d", r.UnreadCount)
		}
	})

	t.Run("unread count falls back to page-local on repo error", func(t *testing.T) {
		// 3 unread in the page; UnreadCount fails. Without a fallback
		// the response would report 0 unread, which would drop the
		// badge to 0 on every transient db hiccup. Page-local count
		// is wrong globally but still > 0 when there's unread work.
		page := []domain.Notification{
			{Id: "1", Type: domain.NotificationLikeType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
			{Id: "2", Type: domain.NotificationReplyType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
			{Id: "3", Type: domain.NotificationFollowType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
			{Id: "4", Type: domain.NotificationLikeType, IsRead: true, UserId: owner, CreatedAt: time.Now()},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				return page, "next", nil
			},
			unreadCountFn: func(userId string) (uint64, error) {
				return 0, errors.New("db boom")
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 3 {
			t.Fatalf("expected page-local fallback count 3, got %d", r.UnreadCount)
		}
	})

	t.Run("since takes precedence and routes to ListSince", func(t *testing.T) {
		var capturedSince string
		var capturedLimit *uint64
		sinceCalled := false
		nots := []domain.Notification{
			{Id: "9", Type: domain.NotificationMentionType, IsRead: false, UserId: owner, CreatedAt: time.Now()},
		}
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				t.Fatal("List must not be called when Since is set")
				return nil, "", nil
			},
			listSinceFn: func(userId, since string, limit *uint64) ([]domain.Notification, string, error) {
				sinceCalled = true
				capturedSince = since
				capturedLimit = limit
				return nots, "end", nil
			},
			unreadCountFn: func(userId string) (uint64, error) { return 1, nil },
		}, stubAuth{owner: domain.Owner{UserId: owner}})

		since := "watermark-7"
		limit := uint64(40)
		resp, err := h(marshal(t, event.GetNotificationsEvent{Since: &since, Limit: &limit}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !sinceCalled {
			t.Fatal("expected ListSince to be called")
		}
		if capturedSince != "watermark-7" {
			t.Fatalf("expected since 'watermark-7', got %q", capturedSince)
		}
		if capturedLimit == nil || *capturedLimit != 40 {
			t.Fatalf("expected limit 40, got %v", capturedLimit)
		}
		r := resp.(event.GetNotificationsResponse)
		if len(r.Notifications) != 1 || r.Notifications[0].Id != "9" {
			t.Fatalf("expected the single delta notification, got %+v", r.Notifications)
		}
	})

	t.Run("empty since falls back to List", func(t *testing.T) {
		listCalled := false
		h := StreamGetNotificationsHandler(stubNotificationRepo{
			listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
				listCalled = true
				return nil, "end", nil
			},
			listSinceFn: func(userId, since string, limit *uint64) ([]domain.Notification, string, error) {
				t.Fatal("ListSince must not be called when Since is empty")
				return nil, "", nil
			},
		}, stubAuth{owner: domain.Owner{UserId: owner}})

		empty := ""
		_, err := h(marshal(t, event.GetNotificationsEvent{Since: &empty}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !listCalled {
			t.Fatal("expected List to be called for empty Since")
		}
	})

	t.Run("with pagination params", func(t *testing.T) {
		var capturedLimit *uint64
		var capturedCursor *string
		h := StreamGetNotificationsHandler(stubNotificationRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
			capturedLimit = limit
			capturedCursor = cursor
			return nil, "end", nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})

		limit := uint64(5)
		cursor := "some-cursor"
		_, err := h(marshal(t, event.GetNotificationsEvent{Limit: &limit, Cursor: &cursor}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if capturedLimit == nil || *capturedLimit != 5 {
			t.Fatalf("expected limit 5, got %v", capturedLimit)
		}
		if capturedCursor == nil || *capturedCursor != "some-cursor" {
			t.Fatalf("expected cursor 'some-cursor', got %v", capturedCursor)
		}
	})
}

func TestStreamGetNotificationHandler(t *testing.T) {
	owner := "owner-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamGetNotificationHandler(stubNotificationRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty notification id", func(t *testing.T) {
		h := StreamGetNotificationHandler(stubNotificationRepo{}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h(marshal(t, event.GetNotificationEvent{}), nil)
		if err == nil {
			t.Fatal("expected error for empty notification id")
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repoErr := errors.New("db failed")
		h := StreamGetNotificationHandler(stubNotificationRepo{getFn: func(userId, notificationId string) (domain.Notification, error) {
			return domain.Notification{}, repoErr
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		_, err := h(marshal(t, event.GetNotificationEvent{NotificationId: "n-1"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		not := domain.Notification{
			Id:        "n-42",
			Type:      domain.NotificationLikeType,
			Text:      "someone liked your tweet",
			UserId:    owner,
			IsRead:    false,
			CreatedAt: time.Now(),
		}
		var capturedUser, capturedId string
		h := StreamGetNotificationHandler(stubNotificationRepo{getFn: func(userId, notificationId string) (domain.Notification, error) {
			capturedUser = userId
			capturedId = notificationId
			return not, nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationEvent{NotificationId: "n-42"}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		got := resp.(event.GetNotificationResponse)
		if got.Id != "n-42" {
			t.Fatalf("expected id n-42, got %s", got.Id)
		}
		if capturedUser != owner {
			t.Fatalf("expected user %s, got %s", owner, capturedUser)
		}
		if capturedId != "n-42" {
			t.Fatalf("expected id n-42, got %s", capturedId)
		}
	})
}
