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
	listFn func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error)
}

func (s stubNotificationRepo) List(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
	if s.listFn != nil {
		return s.listFn(userId, limit, cursor)
	}
	return nil, "", nil
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
		h := StreamGetNotificationsHandler(stubNotificationRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
			return nots, "end", nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
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
		h := StreamGetNotificationsHandler(stubNotificationRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
			return nots, "end", nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
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
		h := StreamGetNotificationsHandler(stubNotificationRepo{listFn: func(userId string, limit *uint64, cursor *string) ([]domain.Notification, string, error) {
			return nots, "end", nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}})
		resp, err := h(marshal(t, event.GetNotificationsEvent{}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		r := resp.(event.GetNotificationsResponse)
		if r.UnreadCount != 0 {
			t.Fatalf("expected 0 unread, got %d", r.UnreadCount)
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
