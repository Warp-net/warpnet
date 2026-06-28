//nolint:all
package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubReportPublisher struct {
	publishFn func(ev event.ReportEvent) error
}

func (s stubReportPublisher) PublishReport(ev event.ReportEvent) error {
	if s.publishFn != nil {
		return s.publishFn(ev)
	}
	return nil
}

func TestStreamReportHandler(t *testing.T) {
	tweetID := "tweet-1"

	mkHandler := func(pub stubReportPublisher) func([]byte, interface{}) (any, error) {
		h := StreamReportHandler(pub)
		return func(buf []byte, _ interface{}) (any, error) { return h(buf, s{}) }
	}

	t.Run("invalid json", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing target_user_id", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetNodeID: "node",
			Reason:       "spam",
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing target_node_id", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			Reason:       "spam",
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty reason", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "",
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("whitespace-only reason rejected", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "   \t\n  ",
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error: whitespace-only reason should not pass")
		}
	})

	t.Run("oversized reason rejected", func(t *testing.T) {
		long := make([]byte, MaxReportReasonLen+1)
		for i := range long {
			long[i] = 'a'
		}
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       string(long),
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error: oversized reason should be rejected")
		}
	})

	t.Run("reply type rejected", func(t *testing.T) {
		objID := domain.ID("some-id")
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "spam",
			Type:         domain.ModerationReplyType,
			ObjectID:     &objID,
		}), nil)
		if err == nil {
			t.Fatal("expected error: reply reports must be rejected end-to-end")
		}
	})

	t.Run("tweet report requires object_id", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "abuse",
			Type:         domain.ModerationTweetType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	// Reason is now a free-form string — the moderator engine decides
	// what to weight, the handler just forwards.
	t.Run("free-form reason is accepted", func(t *testing.T) {
		var got event.ReportEvent
		resp, err := mkHandler(stubReportPublisher{publishFn: func(ev event.ReportEvent) error {
			got = ev
			return nil
		}})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "something the reporter typed",
			Type:         domain.ModerationUserType,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if got.Reason != "something the reporter typed" {
			t.Fatalf("publisher received wrong reason: %+v", got)
		}
	})

	t.Run("tweet report happy path", func(t *testing.T) {
		published := false
		resp, err := mkHandler(stubReportPublisher{publishFn: func(ev event.ReportEvent) error {
			published = true
			return nil
		}})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "illegal",
			Type:         domain.ModerationTweetType,
			ObjectID:     &tweetID,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !published {
			t.Fatal("expected publisher to be called")
		}
	})

	t.Run("publisher error is surfaced", func(t *testing.T) {
		boom := errors.New("publish boom")
		_, err := mkHandler(stubReportPublisher{publishFn: func(ev event.ReportEvent) error {
			return boom
		}})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       "spam",
			Type:         domain.ModerationUserType,
		}), nil)
		if !errors.Is(err, boom) {
			t.Fatalf("expected boom, got: %v", err)
		}
	})
}

func TestStreamReportResultHandler(t *testing.T) {
	owner := "reporter-1"
	tweetID := domain.ID("tweet-1")

	mkHandler := func(notifier stubModerationNotifier, auth stubAuth) func([]byte, interface{}) (any, error) {
		h := StreamReportResultHandler(notifier, auth)
		return func(buf []byte, _ interface{}) (any, error) { return h(buf, s{}) }
	}

	t.Run("invalid payload", func(t *testing.T) {
		h := mkHandler(stubModerationNotifier{}, stubAuth{owner: domain.Owner{UserId: owner}})
		if _, err := h([]byte("{"), nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("tweet moderated - notifies the reporter", func(t *testing.T) {
		var got domain.Notification
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				got = not
				return nil
			}},
			stubAuth{owner: domain.Owner{UserId: owner}},
		)
		reason := "Hate"
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetID,
			UserID:   "offender",
			Result:   domain.FAIL,
			Reason:   &reason,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
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

	t.Run("clean verdict - reporter still told no violation found", func(t *testing.T) {
		var got domain.Notification
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				got = not
				return nil
			}},
			stubAuth{owner: domain.Owner{UserId: owner}},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:     domain.ModerationTweetType,
			ObjectID: &tweetID,
			UserID:   "offender",
			Result:   domain.OK,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if !strings.Contains(got.Text, "no violation") {
			t.Fatalf("expected a no-violation message, got: %q", got.Text)
		}
	})

	t.Run("verdict addressed to a different reporter is dropped", func(t *testing.T) {
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				return nil
			}},
			stubAuth{owner: domain.Owner{UserId: owner}},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:       domain.ModerationUserType,
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
			t.Fatal("must not notify when the verdict names another reporter")
		}
	})

	t.Run("no owner - no notification", func(t *testing.T) {
		notified := false
		h := mkHandler(
			stubModerationNotifier{addFn: func(not domain.Notification) error {
				notified = true
				return nil
			}},
			stubAuth{owner: domain.Owner{}},
		)
		resp, err := h(marshal(t, event.ModerationResultEvent{
			Type:   domain.ModerationUserType,
			UserID: "offender",
			Result: domain.FAIL,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if notified {
			t.Fatal("must not notify when there is no local owner")
		}
	})
}
