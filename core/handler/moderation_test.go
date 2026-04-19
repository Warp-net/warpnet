//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubModerationNotifier struct {
	addFn func(not domain.Notification) error
}

func (s stubModerationNotifier) Add(not domain.Notification) error {
	if s.addFn != nil {
		return s.addFn(not)
	}
	return nil
}

type stubModerationTweetUpdater struct {
	updateFn func(tweet domain.Tweet) error
}

func (s stubModerationTweetUpdater) Update(tweet domain.Tweet) error {
	if s.updateFn != nil {
		return s.updateFn(tweet)
	}
	return nil
}

type stubModerationTimelineDeleter struct {
	deleteFn func(userID, tweetID string) error
}

func (s stubModerationTimelineDeleter) DeleteTweetFromTimeline(userID, tweetID string) error {
	if s.deleteFn != nil {
		return s.deleteFn(userID, tweetID)
	}
	return nil
}

func TestStreamModerationResultHandler(t *testing.T) {
	owner := "owner-1"
	tweetId := "tweet-1"

	t.Run("invalid payload", func(t *testing.T) {
		h := StreamModerationResultHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		_, err := h([]byte("{"), s{})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tweet moderation - missing object id", func(t *testing.T) {
		h := StreamModerationResultHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		_, err := h(marshal(t, event.ModerationResultEvent{Type: domain.ModerationTweetType, UserID: owner}), s{})
		if !errors.Is(err, ErrNoObjectID) {
			t.Fatalf("expected ErrNoObjectID, got: %v", err)
		}
	})

	t.Run("tweet moderation - missing user id", func(t *testing.T) {
		h := StreamModerationResultHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		_, err := h(marshal(t, event.ModerationResultEvent{Type: domain.ModerationTweetType, ObjectID: &tweetId}), s{})
		if !errors.Is(err, ErrNoUserID) {
			t.Fatalf("expected ErrNoUserID, got: %v", err)
		}
	})

	t.Run("tweet moderation OK - no notification", func(t *testing.T) {
		notified := false
		h := StreamModerationResultHandler(stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			return nil
		}}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Result:   domain.OK,
		}), s{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if notified {
			t.Fatal("should not notify on OK result")
		}
	})

	t.Run("tweet moderation FAIL - adds notification for owner", func(t *testing.T) {
		notified := false
		reason := "inappropriate content"
		h := StreamModerationResultHandler(stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			if not.Type != domain.NotificationModerationType {
				t.Fatalf("expected moderation type, got: %v", not.Type)
			}
			return nil
		}}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Result:   domain.FAIL,
			Reason:   &reason,
		}), s{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !notified {
			t.Fatal("expected notification to be added")
		}
	})

	t.Run("tweet moderation FAIL - not owner, no notification", func(t *testing.T) {
		notified := false
		h := StreamModerationResultHandler(stubModerationNotifier{addFn: func(not domain.Notification) error {
			notified = true
			return nil
		}}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   "other-user",
			Result:   domain.FAIL,
		}), s{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if notified {
			t.Fatal("should not notify for other user")
		}
	})

	t.Run("unknown moderation type - returns accepted", func(t *testing.T) {
		h := StreamModerationResultHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:   domain.ModerationObjectType(99),
			UserID: owner,
		}), s{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted for unknown type, got: %v", resp)
		}
	})

	t.Run("tweet moderation - updates tweet and removes from timeline", func(t *testing.T) {
		tweetUpdated := false
		timelineDeleted := false
		h := StreamModerationResultHandler(stubModerationNotifier{}, stubModerationTweetUpdater{updateFn: func(tweet domain.Tweet) error {
			tweetUpdated = true
			if tweet.Moderation == nil {
				t.Fatal("expected moderation info")
			}
			return nil
		}}, stubAuth{owner: domain.Owner{UserId: owner}}, stubModerationTimelineDeleter{deleteFn: func(userID, tweetID string) error {
			timelineDeleted = true
			return nil
		}})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Result:   domain.OK,
		}), s{})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !tweetUpdated {
			t.Fatal("expected tweet to be updated")
		}
		if !timelineDeleted {
			t.Fatal("expected timeline entry to be deleted")
		}
	})
}
