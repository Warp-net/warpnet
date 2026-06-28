//nolint:all
package moderator

import (
	"sync/atomic"
	"testing"

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

// The reporter learns the outcome even on a clean (OK) verdict: the
// moderator opens a direct PUBLIC_POST_REPORT_RESULT stream to their node.
func TestHandleTweetReport_NotifiesReporter(t *testing.T) {
	rec := &recordingEngine{} // OK verdict, stops before isolation
	withEngine(t, rec)

	var (
		gotNode   string
		gotPath   stream.WarpRoute
		gotResult event.ModerationResultEvent
		gotCount  int
	)
	node := stubModeratorNode{
		streamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world"}), nil
			}
			gotNode, gotPath, gotCount = nodeId, path, gotCount+1
			if r, ok := data.(event.ModerationResultEvent); ok {
				gotResult = r
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleTweetReport(rep); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if gotCount != 1 {
		t.Fatalf("expected exactly one report-result delivery, got %d", gotCount)
	}
	if gotPath != event.PUBLIC_POST_REPORT_RESULT {
		t.Fatalf("expected report-result route, got %q", gotPath)
	}
	if gotNode != "reporter-node-1" {
		t.Fatalf("expected delivery to the reporter node, got %q", gotNode)
	}
	if gotResult.ReporterID != "reporter-1" {
		t.Fatalf("expected reporter id propagated, got %q", gotResult.ReporterID)
	}
	if gotResult.Result != domain.OK {
		t.Fatal("expected the OK verdict to be propagated to the reporter")
	}
}

// A report carrying no reporter identity (older client) must not trigger a
// stray delivery — only the tweet fetch goes on the wire.
func TestHandleTweetReport_NoReporterNoDelivery(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	calls := 0
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			calls++
			if path == event.PUBLIC_POST_REPORT_RESULT {
				t.Fatal("must not deliver a report-result without a reporter node id")
			}
			return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world"}), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected only the tweet fetch on the wire, got %d calls", calls)
	}
}
