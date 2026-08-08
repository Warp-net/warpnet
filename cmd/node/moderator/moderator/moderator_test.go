//nolint:all
package moderator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/isolation"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

type stubModeratorNode struct {
	resp []byte
	// streamFn, when set, handles GenericStream per route; falls back to resp.
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

// fixedEngine returns a preset verdict so a test can drive the FAIL path.
type fixedEngine struct {
	ok     bool
	reason string
}

func (e fixedEngine) Moderate(string) (bool, string, error) { return e.ok, e.reason, nil }
func (e fixedEngine) Close()                                {}

type stubPublisher struct{}

func (stubPublisher) PublishUpdateToFollowers(_, _ string, _ any) error { return nil }

func withEngine(t *testing.T, e Engine) {
	t.Helper()
	prev := engine
	engine = e
	t.Cleanup(func() { engine = prev })
	withFastRetry(t)
}

func withFastRetry(t *testing.T) {
	t.Helper()
	prev := fetchRetryDelay
	fetchRetryDelay = 0
	t.Cleanup(func() { fetchRetryDelay = prev })
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

func TestHandleTweetReport_MissingObjectIDIsNoop(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			t.Fatal("must not dial anything without an object id")
			return nil, nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	rep := tweetReport("")
	rep.ObjectID = nil
	if err := m.handleTweetReport(rep); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run without an object id")
	}
}

func TestHandleTweetReport_TransportErrorNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		fetches   int
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				return nil, warpnet.ErrNodeIsOffline
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
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
	if fetches != fetchAttempts {
		t.Fatalf("expected %d fetch attempts, got %d", fetchAttempts, fetches)
	}
	if rec.called {
		t.Fatal("engine must not run when the dial never succeeded")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestHandleTweetReport_ZeroValueTweetNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		fetches   int
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				return []byte(`{}`), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
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
	if fetches != 1 {
		t.Fatalf("a well-formed empty object is not transient, expected 1 fetch, got %d", fetches)
	}
	if rec.called {
		t.Fatal("engine must not run on a zero-value tweet")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestFetchObject_ContextCancelledStopsRetries(t *testing.T) {
	withFastRetry(t)
	fetchRetryDelay = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			fetches++
			return marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"}), nil
		},
	}
	m := &Moderator{ctx: ctx, node: node, isClosed: new(atomic.Bool)}

	_, err := m.fetchObject("node-1", event.PUBLIC_GET_TWEET, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if fetches != 1 {
		t.Fatalf("expected the retry wait to abort after 1 fetch, got %d", fetches)
	}
}

func TestHandleTweetReport_FetchFailureNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		fetches   int
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				return marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
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
	if fetches != fetchAttempts {
		t.Fatalf("expected %d fetch attempts, got %d", fetchAttempts, fetches)
	}
	if rec.called {
		t.Fatal("engine must not run when the content was never fetched")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Result != domain.OK {
		t.Fatal("an unreviewable report must carry an OK (no-op) result")
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestHandleTweetReport_FetchRetriesTransientFailure(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				if fetches < 3 {
					return marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"}), nil
				}
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world"}), nil
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	if err := m.handleTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if fetches != 3 {
		t.Fatalf("expected 3 fetch attempts, got %d", fetches)
	}
	if !rec.called {
		t.Fatal("engine must run once the retry succeeds")
	}
}

func TestHandleTweetReport_AlreadyModeratedReusesVerdict(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	reason := "Violent Crimes"
	var (
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{
					Id:     "tweet-1",
					UserId: "offender",
					Text:   "already judged",
					Moderation: &domain.TweetModeration{
						IsOk:   domain.FAIL,
						Reason: &reason,
					},
				}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(pub),
		isClosed:  new(atomic.Bool),
	}

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleTweetReport(rep); err != nil {
		t.Fatalf("handleTweetReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not re-run on an already moderated tweet")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Result != domain.FAIL {
		t.Fatal("the stored FAIL verdict must be propagated to the reporter")
	}
	if gotResult.Reason == nil || *gotResult.Reason != reason {
		t.Fatalf("the stored reason must be propagated, got %v", gotResult.Reason)
	}
	if pub.calls != 0 {
		t.Fatalf("the followers broadcast already went out on the first verdict, got %d publishes", pub.calls)
	}
}

// On a FAIL verdict the moderator re-sends the result to the reporter's node with ReporterID set.
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

type recordingPublisher struct{ calls int }

func (p *recordingPublisher) PublishUpdateToFollowers(_, _ string, _ any) error {
	p.calls++
	return nil
}

// An OK verdict is delivered to the reporter too (silence reads as "the
// report was lost"), while the followers broadcast stays FAIL-only.
func TestHandleTweetReport_NotifiesReporterOnOkVerdict(t *testing.T) {
	withEngine(t, fixedEngine{ok: true})

	var (
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
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(pub),
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
	if gotResult.Result != domain.OK {
		t.Fatal("expected the OK verdict to be propagated to the reporter")
	}
	if gotResult.ReporterID != "reporter-1" {
		t.Fatalf("expected reporter id propagated, got %q", gotResult.ReporterID)
	}
	if pub.calls != 0 {
		t.Fatalf("an OK verdict must not be broadcast to followers, got %d publishes", pub.calls)
	}
}

// No reporter identity (older client) -> moderated, but no reporter delivery.
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

func userReport() event.ReportEvent {
	return event.ReportEvent{
		Type:           domain.ModerationUserType,
		TargetUserID:   "user-1",
		TargetNodeID:   "node-1",
		Reason:         "Hate",
		ReporterID:     "reporter-1",
		ReporterNodeID: "reporter-node-1",
	}
}

func TestHandleUserReport_ValidProfileReachesEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				return marshal(t, domain.User{Id: "user-1", Username: "troll", Bio: "some bio"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(pub),
		isClosed:  new(atomic.Bool),
	}

	if err := m.handleUserReport(userReport()); err != nil {
		t.Fatalf("handleUserReport: %v", err)
	}
	if !rec.called {
		t.Fatal("engine must run on a real profile")
	}
	if rec.text != "troll\nsome bio" {
		t.Fatalf("engine got %q, want the concatenated profile text", rec.text)
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Result != domain.OK {
		t.Fatal("expected the OK verdict propagated to the reporter")
	}
	if pub.calls != 0 {
		t.Fatalf("an OK verdict must not be broadcast to followers, got %d publishes", pub.calls)
	}
}

func TestHandleUserReport_FailVerdictIsolatesAndNotifies(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	var (
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				return marshal(t, domain.User{Id: "user-1", Username: "troll", Bio: "some bio"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m := &Moderator{
		node:      node,
		isolation: isolation.NewIsolationProtocol(pub),
		isClosed:  new(atomic.Bool),
	}

	if err := m.handleUserReport(userReport()); err != nil {
		t.Fatalf("handleUserReport: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Result != domain.FAIL {
		t.Fatal("expected the FAIL verdict propagated to the reporter")
	}
	if pub.calls == 0 {
		t.Fatal("a FAIL verdict must be broadcast to the offender's followers")
	}
}

func TestHandleUserReport_FetchFailureNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		fetches   int
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				fetches++
				return marshal(t, event.ResponseError{Code: 500, Message: "user not found"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	if err := m.handleUserReport(userReport()); err != nil {
		t.Fatalf("handleUserReport: %v", err)
	}
	if fetches != fetchAttempts {
		t.Fatalf("expected %d fetch attempts, got %d", fetchAttempts, fetches)
	}
	if rec.called {
		t.Fatal("engine must not run when the profile was never fetched")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Result != domain.OK {
		t.Fatal("an unreviewable report must carry an OK (no-op) result")
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestHandleUserReport_ZeroValueUserNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				return []byte(`{}`), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	if err := m.handleUserReport(userReport()); err != nil {
		t.Fatalf("handleUserReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on a zero-value user")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestHandleUserReport_EmptyProfileTextNotifiesReporter(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	var (
		delivered int
		gotResult event.ModerationResultEvent
	)
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				return marshal(t, domain.User{Id: "user-1"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationResultEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{node: node, isClosed: new(atomic.Bool)}

	if err := m.handleUserReport(userReport()); err != nil {
		t.Fatalf("handleUserReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on an empty profile text")
	}
	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Reason == nil || *gotResult.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", gotResult.Reason)
	}
}

func TestHandleReport_ClosedModeratorIsNoop(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			t.Fatal("a closed moderator must not dial anything")
			return nil, nil
		},
	}
	closed := new(atomic.Bool)
	closed.Store(true)
	m := &Moderator{node: node, isClosed: closed}

	if err := m.handleReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on a closed moderator")
	}
}
