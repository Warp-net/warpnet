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
			Reason:       domain.ReportReasonSpam,
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing target_node_id", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			Reason:       domain.ReportReasonSpam,
			Type:         domain.ModerationUserType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid reason", func(t *testing.T) {
		_, err := mkHandler(stubReportPublisher{})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       domain.ReportReason("bogus"),
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
			Reason:       domain.ReportReasonAbuse,
			Type:         domain.ModerationTweetType,
		}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("user report happy path", func(t *testing.T) {
		var got event.ReportEvent
		resp, err := mkHandler(stubReportPublisher{publishFn: func(ev event.ReportEvent) error {
			got = ev
			return nil
		}})(marshal(t, event.ReportEvent{
			TargetUserID: "user",
			TargetNodeID: "node",
			Reason:       domain.ReportReasonNSFW,
			Type:         domain.ModerationUserType,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != event.Accepted {
			t.Fatalf("expected accepted, got: %v", resp)
		}
		if got.TargetUserID != "user" || got.Reason != domain.ReportReasonNSFW {
			t.Fatalf("publisher received wrong event: %+v", got)
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
			Reason:       domain.ReportReasonIllegal,
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
			Reason:       domain.ReportReasonSpam,
			Type:         domain.ModerationUserType,
		}), nil)
		if !errors.Is(err, boom) {
			t.Fatalf("expected boom, got: %v", err)
		}
	})
}
