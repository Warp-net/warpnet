//nolint:all
package handler

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
)

// moderatorTestKey returns a deterministic moderator identity whose peer id
// embeds the pubkey, the way real node ids do.
func moderatorTestKey(t *testing.T, seed string) (ed25519.PrivateKey, string) {
	t.Helper()
	priv, err := security.GenerateKeyFromSeed([]byte(seed))
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return priv, id.String()
}

// signedResult stamps a verdict with a valid moderator identity and
// signature, exactly the way a moderator ships one.
func signedResult(t *testing.T, ev event.ModerationVerdictEvent) []byte {
	t.Helper()
	priv, id := moderatorTestKey(t, "moderation-handler-test")
	ev.ModeratorID = id
	return marshal(t, ev.Signed(priv))
}

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

	// mustNotTouchState wires stubs that fail the test on any write or
	// notification: a rejected verdict must leave the node untouched.
	mustNotTouchState := func(t *testing.T) func([]byte, interface{}) (any, error) {
		t.Helper()
		return mkHandler(
			stubModerationNotifier{addFn: func(domain.Notification) error {
				t.Fatal("rejected verdict must not notify anyone")
				return nil
			}},
			stubModerationTweetUpdater{updateFn: func(domain.Tweet) error {
				t.Fatal("rejected verdict must not update tweets")
				return nil
			}},
			stubModerationUserUpdater{updateFn: func(_ string, u domain.User) (domain.User, error) {
				t.Fatal("rejected verdict must not update users")
				return u, nil
			}},
			stubModerationTimelineDeleter{deleteFn: func(_, _ string) error {
				t.Fatal("rejected verdict must not touch the timeline")
				return nil
			}},
		)
	}

	t.Run("unsigned verdict is dropped", func(t *testing.T) {
		h := mustNotTouchState(t)
		_, id := moderatorTestKey(t, "moderation-handler-test")
		_, err := h(marshal(t, event.ModerationVerdictEvent{
			Type:        domain.ModerationTweetType,
			ObjectID:    &tweetId,
			UserID:      "offender",
			Verdict:     domain.FAIL,
			ModeratorID: id,
			ReporterID:  owner,
		}), nil)
		if !errors.Is(err, ErrBadModeratorSignature) {
			t.Fatalf("expected ErrBadModeratorSignature, got: %v", err)
		}
	})

	t.Run("verdict with malformed moderator id is dropped", func(t *testing.T) {
		h := mustNotTouchState(t)
		_, err := h(marshal(t, event.ModerationVerdictEvent{
			Type:        domain.ModerationTweetType,
			ObjectID:    &tweetId,
			UserID:      "offender",
			Verdict:     domain.FAIL,
			ModeratorID: "not-a-peer-id",
		}), nil)
		if !errors.Is(err, ErrNoModeratorID) {
			t.Fatalf("expected ErrNoModeratorID, got: %v", err)
		}
	})

	t.Run("tampered verdict is dropped", func(t *testing.T) {
		h := mustNotTouchState(t)
		priv, id := moderatorTestKey(t, "moderation-handler-test")
		ev := event.ModerationVerdictEvent{
			Type:        domain.ModerationTweetType,
			ObjectID:    &tweetId,
			UserID:      "offender",
			Verdict:     domain.OK,
			ModeratorID: id,
			TimeAt:      time.Now().UTC(),
		}
		ev = ev.Signed(priv)
		ev.Verdict = domain.FAIL // flip after signing
		_, err := h(marshal(t, ev), nil)
		if !errors.Is(err, ErrBadModeratorSignature) {
			t.Fatalf("expected ErrBadModeratorSignature, got: %v", err)
		}
	})

	t.Run("verdict signed by an impostor key is dropped", func(t *testing.T) {
		h := mustNotTouchState(t)
		impostor, _ := moderatorTestKey(t, "impostor")
		_, claimedID := moderatorTestKey(t, "moderation-handler-test")
		ev := event.ModerationVerdictEvent{
			Type:        domain.ModerationTweetType,
			ObjectID:    &tweetId,
			UserID:      "offender",
			Verdict:     domain.FAIL,
			ModeratorID: claimedID, // claims someone else's identity
			TimeAt:      time.Now().UTC(),
		}
		ev = ev.Signed(impostor)
		_, err := h(marshal(t, ev), nil)
		if !errors.Is(err, ErrBadModeratorSignature) {
			t.Fatalf("expected ErrBadModeratorSignature, got: %v", err)
		}
	})

	t.Run("tweet moderation - missing object id", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		_, err := h(signedResult(t, event.ModerationVerdictEvent{Type: domain.ModerationTweetType, UserID: owner}), nil)
		if !errors.Is(err, ErrNoObjectID) {
			t.Fatalf("expected ErrNoObjectID, got: %v", err)
		}
	})

	t.Run("tweet moderation - missing user id", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubModerationTweetUpdater{}, stubModerationUserUpdater{}, stubModerationTimelineDeleter{})
		_, err := h(signedResult(t, event.ModerationVerdictEvent{Type: domain.ModerationTweetType, ObjectID: &tweetId}), nil)
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Verdict:  domain.FAIL,
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Verdict:    domain.FAIL,
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
		if got.RecepientId != owner {
			t.Fatalf("notification must target the reporter, got %q", got.RecepientId)
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Verdict:    domain.FAIL,
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   "other-user",
			Verdict:  domain.FAIL,
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   owner,
			Verdict:  domain.FAIL,
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

	t.Run("tweet moderation FAIL - deletes from the local owner's timeline", func(t *testing.T) {
		var gotUserID, gotTweetID string
		h := mkHandler(
			stubModerationNotifier{},
			stubModerationTweetUpdater{},
			stubModerationUserUpdater{},
			stubModerationTimelineDeleter{deleteFn: func(userID, tweetID string) error {
				gotUserID, gotTweetID = userID, tweetID
				return nil
			}},
		)
		_, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetId,
			UserID:   "offender",
			Verdict:  domain.FAIL,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if gotUserID != owner {
			t.Fatalf("timeline delete must use the local owner's id, got %q", gotUserID)
		}
		if gotTweetID != tweetId {
			t.Fatalf("expected tweet %q deleted, got %q", tweetId, gotTweetID)
		}
	})

	t.Run("unreviewable report - notifies reporter honestly", func(t *testing.T) {
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
		reason := event.ModerationReasonUnavailable
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Verdict:    domain.OK,
			Reason:     &reason,
			ReporterID: owner,
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
		if !strings.Contains(got.Text, "could not be reviewed") {
			t.Fatalf("expected 'could not be reviewed' wording, got: %q", got.Text)
		}
		if strings.Contains(got.Text, "no violation") {
			t.Fatalf("a fetch failure must not read as a clean verdict: %q", got.Text)
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:       domain.ModerationTweetType,
			ObjectID:   &tweetId,
			UserID:     "offender",
			Verdict:    domain.OK,
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:    domain.ModerationUserType,
			UserID:  target,
			Verdict: domain.FAIL,
			Reason:  &reason,
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
		resp, err := h(signedResult(t, event.ModerationVerdictEvent{
			Type:    domain.ModerationUserType,
			UserID:  target,
			Verdict: domain.FAIL,
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
