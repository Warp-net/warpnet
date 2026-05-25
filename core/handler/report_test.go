//nolint:all
package handler

import (
	"errors"
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
