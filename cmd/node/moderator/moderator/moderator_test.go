//nolint:all
package moderator

import (
	"sync/atomic"
	"testing"

	"github.com/Warp-net/warpnet/cmd/node/moderator/isolation"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubModeratorNode struct {
	resp []byte
	// streamFn, when set, handles every GenericStream call so a test can
	// return different payloads per route (fetch the tweet, then capture
	// the report-result delivery). Falls back to resp otherwise.
	streamFn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

func (s stubModeratorNode) Node() warpnet.P2PNode      { return nil }
func (s stubModeratorNode) ID() warpnet.WarpPeerID     { return "" }
func (s stubModeratorNode) NodeInfo() warpnet.NodeInfo { return warpnet.NodeInfo{} }
func (s stubModeratorNode) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.streamFn != nil {
		return s.streamFn(nodeId, path, data)
	}
	return s.resp, nil
}

type recordingEngine struct {
	called bool
	text   string
}

// Moderate returns an ok verdict so the report stops before isolation,
// keeping the test focused on the fetch/parse path.
func (e *recordingEngine) Moderate(content string) (bool, string, error) {
	e.called = true
	e.text = content
	return true, "", nil
}

func (e *recordingEngine) Close() {}

// fixedEngine returns a preset verdict, letting a test drive the FAIL path
// (which the OK-returning recordingEngine deliberately stops short of).
type fixedEngine struct {
	ok     bool
	reason string
}

func (e fixedEngine) Moderate(string) (bool, string, error) { return e.ok, e.reason, nil }
func (e fixedEngine) Close()                                {}

// stubPublisher satisfies the isolation Publisher; the followers/observers
// broadcast is a no-op so tests can focus on the direct reporter delivery.
type stubPublisher struct{}

func (stubPublisher) PublishUpdateToFollowers(_, _ string, _ any) error { return nil }

func withEngine(t *testing.T, e Engine) {
	t.Helper()
	prev := engine
	engine = e
	t.Cleanup(func() { engine = prev })
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func tweetReport(objectID string) event.ReportEvent {
	id := domain.ID(objectID)
	return event.ReportEvent{
		Type:         domain.ModerationTweetType,
		TargetUserID: "user-1",
		TargetNodeID: "node-1",
		ObjectID:     &id,
		Reason:       "Hate",
	}
}

// A failed fetch comes back as an event.ResponseError envelope, not a
// transport error. It must not be parsed into a zero-value tweet and fed
// to the engine.
func TestHandleTweetReport_ErrorResponseSkipsEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"})},
		isClosed: new(atomic.Bool),
	}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on an error response")
	}
}

func TestHandleTweetReport_ValidTweetReachesEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world"})},
		isClosed: new(atomic.Bool),
	}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if !rec.called {
		t.Fatal("engine must run on a real tweet")
	}
	if rec.text != "hello world" {
		t.Fatalf("engine got %q, want %q", rec.text, "hello world")
	}
}

func TestHandleTweetReport_EmptyTextSkipsEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: ""})},
		isClosed: new(atomic.Bool),
	}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if rec.called {
		t.Fatal("image-only tweet has no text to moderate")
	}
}

// On an actioned (FAIL) verdict the moderator re-sends the result straight
// to the reporter's node on PUBLIC_POST_MODERATION_RESULT, with ReporterID
// set so that node raises a notification for the reporter.
func TestHandleTweetReport_NotifiesReporter(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	var (
		gotNode   string
		gotResult event.ModerationResultEvent
		delivered int
	)
	node := stubModeratorNode{
		streamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				gotNode = nodeId
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(stubPublisher{}),
		isClosed:  new(atomic.Bool),
	}

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleTweetReport(rep); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotNode != "reporter-node-1" {
		t.Fatalf("expected delivery to the reporter node, got %q", gotNode)
	}
	if gotResult.ReporterID != "reporter-1" {
		t.Fatalf("expected reporter id propagated, got %q", gotResult.ReporterID)
	}
	if gotResult.Result != domain.FAIL {
		t.Fatal("expected the FAIL verdict to be propagated to the reporter")
	}
}

// A report carrying no reporter identity (older client) is still moderated
// but must not trigger a direct reporter delivery.
func TestHandleTweetReport_NoReporterNoDelivery(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				t.Fatal("must not deliver to a reporter without a reporter node id")
			}
			return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
		},
	}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(stubPublisher{}),
		isClosed:  new(atomic.Bool),
	}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
}
