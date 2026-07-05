//nolint:all
package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/database"
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

type stubModerationUserUpdater struct {
	getFn    func(userId string) (domain.User, error)
	updateFn func(userId string, user domain.User) (domain.User, error)
}

func (s stubModerationUserUpdater) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{Id: userId}, nil
}

func (s stubModerationUserUpdater) Update(userId string, user domain.User) (domain.User, error) {
	if s.updateFn != nil {
		return s.updateFn(userId, user)
	}
	return user, nil
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
	target := "target-1"

	mkHandler := func(
		notifier stubModerationNotifier,
		tweets stubModerationTweetUpdater,
		users stubModerationUserUpdater,
		timeline stubModerationTimelineDeleter,
	) func([]byte, interface{}) (any, error) {
		h := StreamModerationResultHandler(notifier, tweets, users, timeline, stubAuth{owner: domain.Owner{UserId: owner}})
		return func(buf []byte, _ interface{}) (any, error) { return h(buf, s{}) }
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		_, err := h([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tweet moderation - missing object id", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		_, err := h(marshal(t, event.ModerationResultEvent{Type: domain.ModerationTweetType, UserID: owner}), nil)
		if !errors.Is(err, ErrNoObjectID) {
			t.Fatalf("expected ErrNoObjectID, got: %v", err)
		}
	})

	t.Run("tweet moderation - missing user id", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		_, err := h(marshal(t, event.ModerationResultEvent{Type: domain.ModerationTweetType, ObjectID: &tweetId}), nil)
		if !errors.Is(err, ErrNoUserID) {
			t.Fatalf("expected ErrNoUserID, got: %v", err)
		}
	})

	// Shadow-ban semantics: the offender's node never receives a
	// moderation stream, so the handler must never trigger a
	// user-facing notification — not on OK, not on FAIL, not when the
	// verdict happens to mention the local owner. The notification
	// branch used to fire when `ev.UserID == owner.UserId`; that branch
	// is gone.
	t.Run("tweet moderation FAIL for local owner - still no notification (shadow ban)", func(t *testing.T) {
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				return nil
			}},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{},
		)
		reason := "inappropriate content"
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Result:   domain.FAIL,
			Reason:   &reason,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if notified {
			t.Fatal("offender must NOT be notified (shadow-ban semantics)")
		}
	})

	// ReporterID set + matching the local owner turns the same handler into a notifying one.
	t.Run("reported tweet actioned - notifies the reporter", func(t *testing.T) {
		var got domain.Notification
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				got = not
				return nil
			}},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{},
		)
		reason := "Hate"
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Result:     domain.FAIL,
			Reason:     &reason,
			ReporterID: owner, // delivered straight to the reporter
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !notified {
			t.Fatal("the reporter must be notified")
		}
		if got.UserId != owner {
			t.Fatalf("notification must target the reporter, got %q", got.UserId)
		}
		if got.Type != domain.NotificationModerationType {
			t.Fatalf("expected moderation notification, got %q", got.Type)
		}
		if !strings.Contains(got.Text, "tweet") || !strings.Contains(got.Text, "Hate") {
			t.Fatalf("unexpected notification text: %q", got.Text)
		}
	})

	t.Run("verdict naming another reporter is not notified locally", func(t *testing.T) {
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				return nil
			}},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Result:     domain.FAIL,
			ReporterID: "someone-else",
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if notified {
			t.Fatal("must not notify when the verdict names a different reporter")
		}
	})

	t.Run("tweet moderation FAIL for other user - no notification", func(t *testing.T) {
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				return nil
			}},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   "other-user",
			Result:   domain.FAIL,
		}), nil)
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
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:   domain.ModerationObjectType(99),
			UserID: owner,
		}), nil)
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
		h := mkHandler(
			stubModerationNotifier{},
			stubModerationTweetUpdater{updateFn: func(tweet domain.Tweet) error {
				tweetUpdated = true
				if tweet.Moderation == nil {
					t.Fatal("expected moderation info")
				}
				return nil
			}},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{deleteFn: func(userID, tweetID string) error {
				timelineDeleted = true
				return nil
			}},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Result:   domain.FAIL,
		}), nil)
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

	// An OK verdict arrives only on the reporter-bound delivery. It must
	// notify the reporter with the "no violation" wording and leave the
	// local tweet and timeline untouched.
	t.Run("reported tweet cleared - notifies reporter, no isolation", func(t *testing.T) {
		var got domain.Notification
		notified := false
		tweetUpdated := false
		timelineDeleted := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				got = not
				return nil
			}},
			stubModerationTweetUpdater{updateFn: func(tweet domain.Tweet) error {
				tweetUpdated = true
				return nil
			}},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{deleteFn: func(userID, tweetID string) error {
				timelineDeleted = true
				return nil
			}},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Result:     domain.OK,
			ReporterID: owner,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !notified {
			t.Fatal("the reporter must be notified about the OK verdict")
		}
		if !strings.Contains(got.Text, "no violation") {
			t.Fatalf("expected 'no violation' wording, got: %q", got.Text)
		}
		if tweetUpdated {
			t.Fatal("an OK verdict must not update the local tweet")
		}
		if timelineDeleted {
			t.Fatal("an OK verdict must not delete anything from the timeline")
		}
	})

	// New: profile-level moderation marks the user row and never errors
	// out when the user isn't cached locally (observer doesn't follow).
	t.Run("user moderation - sets user.Moderation flag", func(t *testing.T) {
		var updated domain.User
		h := mkHandler(
			stubModerationNotifier{},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{
				getFn: func(userId string) (domain.User, error) {
					return domain.User{Id: userId, Bio: "old bio"}, nil
				},
				updateFn: func(userId string, user domain.User) (domain.User, error) {
					updated = user
					return user, nil
				},
			},
			stubModerationTimelineDeleter{},
		)
		reason := "abuse"
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:   domain.ModerationUserType,
			UserID: target,
			Result: domain.FAIL,
			Reason: &reason,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if updated.Moderation == nil || !updated.Moderation.IsModerated {
			t.Fatalf("expected user moderation flag set: %+v", updated.Moderation)
		}
		if updated.Bio != "old bio" {
			t.Fatalf("Bio must not be wiped — UI hides it on the flag, not the storage: got %q", updated.Bio)
		}
	})

	t.Run("user moderation - unknown user is a no-op", func(t *testing.T) {
		updateCalled := false
		h := mkHandler(
			stubModerationNotifier{},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{
				getFn: func(userId string) (domain.User, error) {
					return domain.User{}, database.ErrUserNotFound
				},
				updateFn: func(userId string, user domain.User) (domain.User, error) {
					updateCalled = true
					return user, nil
				},
			},
			stubModerationTimelineDeleter{},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:   domain.ModerationUserType,
			UserID: target,
			Result: domain.FAIL,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if updateCalled {
			t.Fatal("update must not be called when the user isn't cached")
		}
	})
}
