//nolint:all
package moderator

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/round"
	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
)

type stubModeratorNode struct {
	id   string
	resp []byte
	// streamFn, when set, handles GenericStream per route; falls back to resp.
	streamFn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

func (s stubModeratorNode) Node() warpnet.P2PNode      { return nil }
func (s stubModeratorNode) ID() warpnet.WarpPeerID     { return warpnet.WarpPeerID(s.id) }
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

// Moderate returns an ok verdict so the assessment stops before isolation,
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

type recordingPublisher struct{ calls int }

func (p *recordingPublisher) PublishUpdateToFollowers(_, _ string, _ any) error {
	p.calls++
	return nil
}

// stubVotes is an in-process VoteExchange: published votes land on a channel
// the test can wait on, which also gives happens-before for the closure
// counters written inside the volunteer-timer goroutine.
type stubVotes struct {
	published chan vote.Event
}

func (s *stubVotes) PublishVote(ev vote.Event) error {
	if s.published != nil {
		s.published <- ev
	}
	return nil
}

func (s *stubVotes) SubscribeVotes(func(ev vote.Event) error) error { return nil }

func withEngine(t *testing.T, e Engine) {
	t.Helper()
	prev := engine
	engine = e
	t.Cleanup(func() { engine = prev })
}

// fastRetrier keeps the retry semantics of the real one without the wait.
// Not zero: the retrier's jitter draws from minInterval/2 and panics on an
// empty range.
func fastRetrier() retrier.Retrier {
	return retrier.New(time.Millisecond, fetchAttempts, retrier.FixedBackoff)
}

// newTestModerator builds a moderator with its round registry frozen: the
// voting protocol has its own tests in the round package, so nothing here
// should depend on a timer firing.
func newTestModerator(t *testing.T, node ModeratorNode, pub Publisher, privKey ed25519.PrivateKey) (*Moderator, *stubVotes) {
	t.Helper()
	votes := &stubVotes{published: make(chan vote.Event, 8)}
	m, err := NewModerator(context.Background(), node, pub, nil, votes, privKey)
	if err != nil {
		t.Fatalf("NewModerator: %v", err)
	}
	m.rounds = round.NewRegistry(m.selfID(), m, round.Schedule{
		Window: time.Hour, Failover: time.Hour, Step: time.Hour,
	})
	return m, votes
}

func waitVote(t *testing.T, votes *stubVotes) vote.Event {
	t.Helper()
	select {
	case v := <-votes.published:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("vote was never published")
		return vote.Event{}
	}
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

func assertUnavailable(t *testing.T, a assessment) {
	t.Helper()
	if a.text != "" {
		t.Fatalf("nothing was judged, so no reference may be kept: %q", a.text)
	}
	v := a.ballot
	if v.Result != domain.OK {
		t.Fatal("an unreviewable report must carry an OK (no-op) result")
	}
	if v.Reason == nil || *v.Reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", v.Reason)
	}
}

// A failed fetch comes back as an event.ResponseError envelope, not a
// transport error. It must not be parsed into a zero-value tweet and fed
// to the engine.
func TestAssessTweetReport_ErrorResponseSkipsEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"})},
		ctx:      context.Background(),
		retrier:  fastRetrier(),
		isClosed: new(atomic.Bool),
	}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	if rec.called {
		t.Fatal("engine must not run on an error response")
	}
	assertUnavailable(t, v)
}

func TestAssessTweetReport_ValidTweetReachesEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"})},
		ctx:      context.Background(),
		retrier:  fastRetrier(),
		isClosed: new(atomic.Bool),
	}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if !ok {
		t.Fatal("a real tweet must yield a vote")
	}
	if !rec.called {
		t.Fatal("engine must run on a real tweet")
	}
	if rec.text != "hello world" {
		t.Fatalf("engine got %q, want %q", rec.text, "hello world")
	}
	if v.ballot.Result != domain.OK {
		t.Fatal("recordingEngine clears content, expected an OK verdict")
	}
	if v.ballot.ObjectID == nil || *v.ballot.ObjectID != "tweet-1" || v.ballot.UserID != "offender" {
		t.Fatalf("verdict must carry the tweet identifiers, got %+v", v)
	}
}

func TestAssessTweetReport_EmptyTextSkipsEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{
		node:     stubModeratorNode{resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: ""})},
		ctx:      context.Background(),
		retrier:  fastRetrier(),
		isClosed: new(atomic.Bool),
	}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if !ok {
		t.Fatal("an image-only tweet still yields a vote")
	}
	if rec.called {
		t.Fatal("image-only tweet has no text to moderate")
	}
	assertUnavailable(t, v)
}

func TestAssessTweetReport_MissingObjectIDIsNoVote(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			t.Fatal("must not dial anything without an object id")
			return nil, nil
		},
	}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	rep := tweetReport("")
	rep.ObjectID = nil
	_, ok, err := m.assessTweetReport(rep)
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if ok {
		t.Fatal("a malformed report must not yield a vote")
	}
	if rec.called {
		t.Fatal("engine must not run without an object id")
	}
}

func TestAssessTweetReport_TransportErrorYieldsUnavailable(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				return nil, warpnet.ErrNodeIsOffline
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if fetches != fetchAttempts {
		t.Fatalf("expected %d fetch attempts, got %d", fetchAttempts, fetches)
	}
	if rec.called {
		t.Fatal("engine must not run when the dial never succeeded")
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	assertUnavailable(t, v)
}

func TestAssessTweetReport_ZeroValueTweetYieldsUnavailable(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				fetches++
				return []byte(`{}`), nil
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if fetches != 1 {
		t.Fatalf("a well-formed empty object is not transient, expected 1 fetch, got %d", fetches)
	}
	if rec.called {
		t.Fatal("engine must not run on a zero-value tweet")
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	assertUnavailable(t, v)
}

func TestFetch_CancelledContextStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			fetches++
			return marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"}), nil
		},
	}
	m := &Moderator{
		ctx:      ctx,
		node:     node,
		retrier:  fastRetrier(),
		isClosed: new(atomic.Bool),
	}

	_, err := m.fetch("node-1", event.PUBLIC_GET_TWEET, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if fetches != 0 {
		t.Fatalf("a cancelled context must abort before dialing, got %d fetches", fetches)
	}
}

// An application error envelope is a retryable failure, not an object: the
// caller must never parse {"code":500,...} into a zero-value tweet.
func TestFetch_ErrorEnvelopeIsRetriedThenFails(t *testing.T) {
	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			fetches++
			return marshal(t, event.ResponseError{Code: 500, Message: "tweet not found"}), nil
		},
	}
	m := &Moderator{
		ctx:      context.Background(),
		node:     node,
		retrier:  fastRetrier(),
		isClosed: new(atomic.Bool),
	}

	data, err := m.fetch("node-1", event.PUBLIC_GET_TWEET, nil)
	if err == nil {
		t.Fatal("an error envelope must surface as a fetch failure")
	}
	if data != nil {
		t.Fatalf("no payload may be returned on failure, got %s", data)
	}
	if fetches != fetchAttempts {
		t.Fatalf("expected %d attempts, got %d", fetchAttempts, fetches)
	}
}

func TestAssessTweetReport_FetchRetriesTransientFailure(t *testing.T) {
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
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	if _, _, err := m.assessTweetReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if fetches != 3 {
		t.Fatalf("expected 3 fetch attempts, got %d", fetches)
	}
	if !rec.called {
		t.Fatal("engine must run once the retry succeeds")
	}
}

func TestAssessTweetReport_AlreadyModeratedReusesVerdict(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	reason := "Violent Crimes"
	node := stubModeratorNode{
		resp: marshal(t, domain.Tweet{
			Id:     "tweet-1",
			UserId: "offender",
			Text:   "already judged",
			Moderation: &domain.TweetModeration{
				IsOk:   domain.FAIL,
				Reason: &reason,
			},
		}),
	}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessTweetReport(tweetReport("tweet-1"))
	if err != nil {
		t.Fatalf("assessTweetReport: %v", err)
	}
	if !ok {
		t.Fatal("an already moderated tweet still yields a vote")
	}
	if rec.called {
		t.Fatal("engine must not re-run on an already moderated tweet")
	}
	if v.ballot.Result != domain.FAIL {
		t.Fatal("the stored FAIL verdict must be reused")
	}
	if v.ballot.Reason == nil || *v.ballot.Reason != reason {
		t.Fatalf("the stored reason must be reused, got %v", v.ballot.Reason)
	}
}

func TestAssessUserReport_ValidProfileReachesEngine(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{resp: marshal(t, domain.User{Id: "user-1", Username: "troll", Bio: "some bio"})}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessUserReport(userReport())
	if err != nil {
		t.Fatalf("assessUserReport: %v", err)
	}
	if !ok {
		t.Fatal("a real profile must yield a vote")
	}
	if !rec.called {
		t.Fatal("engine must run on a real profile")
	}
	if rec.text != "troll\nsome bio" {
		t.Fatalf("engine got %q, want the concatenated profile text", rec.text)
	}
	if v.ballot.Result != domain.OK || v.ballot.UserID != "user-1" {
		t.Fatalf("unexpected verdict %+v", v)
	}
}

func TestAssessUserReport_FetchFailureYieldsUnavailable(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	fetches := 0
	node := stubModeratorNode{
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_USER {
				fetches++
				return marshal(t, event.ResponseError{Code: 500, Message: "user not found"}), nil
			}
			return []byte(event.Accepted), nil
		},
	}
	m := &Moderator{ctx: context.Background(), node: node, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessUserReport(userReport())
	if err != nil {
		t.Fatalf("assessUserReport: %v", err)
	}
	if fetches != fetchAttempts {
		t.Fatalf("expected %d fetch attempts, got %d", fetchAttempts, fetches)
	}
	if rec.called {
		t.Fatal("engine must not run when the profile was never fetched")
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	assertUnavailable(t, v)
}

func TestAssessUserReport_ZeroValueUserYieldsUnavailable(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{ctx: context.Background(), node: stubModeratorNode{resp: []byte(`{}`)}, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessUserReport(userReport())
	if err != nil {
		t.Fatalf("assessUserReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on a zero-value user")
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	assertUnavailable(t, v)
}

func TestAssessUserReport_EmptyProfileTextYieldsUnavailable(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	m := &Moderator{ctx: context.Background(), node: stubModeratorNode{resp: marshal(t, domain.User{Id: "user-1"})}, retrier: fastRetrier(), isClosed: new(atomic.Bool)}

	v, ok, err := m.assessUserReport(userReport())
	if err != nil {
		t.Fatalf("assessUserReport: %v", err)
	}
	if rec.called {
		t.Fatal("engine must not run on an empty profile text")
	}
	if !ok {
		t.Fatal("an unreviewable report still yields a vote")
	}
	assertUnavailable(t, v)
}

// --- round.Participant implementation ---

// Ballot stamps the round identity onto whatever the assessment produced,
// so the round itself never has to know how a ballot is built.
func TestBallot_StampsRoundIdentity(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	node := stubModeratorNode{
		id:   "mod-self",
		resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}),
	}
	m, _ := newTestModerator(t, node, stubPublisher{}, nil)

	v, ok, err := m.Ballot("round-42", tweetReport("tweet-1"))
	if err != nil || !ok {
		t.Fatalf("Ballot: %v ok=%t", err, ok)
	}
	if v.ReportID != "round-42" {
		t.Fatalf("the ballot must carry the round id, got %q", v.ReportID)
	}
	if v.ModeratorID != m.selfID() {
		t.Fatalf("the ballot must be cast under this node's id, got %q", v.ModeratorID)
	}
	if v.Type != domain.ModerationTweetType || v.Result != domain.FAIL {
		t.Fatalf("unexpected ballot %+v", v)
	}
}

func TestBallot_UnsupportedTypeAbstains(t *testing.T) {
	withEngine(t, fixedEngine{ok: false})

	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	rep.Type = domain.ModerationImageType
	if _, ok, err := m.Ballot("round-1", rep); ok || err != nil {
		t.Fatalf("an unsupported report type must abstain, got ok=%t err=%v", ok, err)
	}
}

// decidedFixture drives Decided against a recording node and publisher.
func decidedFixture(t *testing.T, privKey ed25519.PrivateKey) (*Moderator, *recordingPublisher, *[]event.ModerationVerdictEvent, *[]string) {
	t.Helper()
	delivered := new([]event.ModerationVerdictEvent)
	deliveredTo := new([]string)
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(nodeID string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					*delivered = append(*delivered, r)
					*deliveredTo = append(*deliveredTo, nodeID)
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, _ := newTestModerator(t, node, pub, privKey)
	return m, pub, delivered, deliveredTo
}

func decidedBallot(result domain.ModerationResult, reason string) vote.Event {
	objectID := domain.ID("tweet-1")
	return vote.Event{
		ReportID: "round-1", Type: domain.ModerationTweetType,
		Result: result, Reason: &reason, UserID: "offender", ObjectID: &objectID,
		ModeratorID: "whoever-was-chair",
	}
}

func TestDecided_FailNotifiesReporterAndIsolates(t *testing.T) {
	privKey, err := security.GenerateKeyFromSeed([]byte("decided-signs"))
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	m, pub, delivered, deliveredTo := decidedFixture(t, privKey)

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	voters := []domain.ID{"mod-a", "mod-b", "mod-c"}

	m.Decided(rep, decidedBallot(domain.FAIL, "Hate"), voters)

	if len(*delivered) != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", len(*delivered))
	}
	got := (*delivered)[0]
	if (*deliveredTo)[0] != "reporter-node-1" || got.ReporterID != "reporter-1" {
		t.Fatalf("the verdict must be addressed to the reporter, got %q / %+v", (*deliveredTo)[0], got)
	}
	if got.Verdict != domain.FAIL || len(got.Voters) != 3 {
		t.Fatalf("the verdict must carry the outcome and its voters, got %+v", got)
	}
	// The verdict goes out under THIS node's identity, not the ballot's.
	if got.ModeratorID != m.selfID() {
		t.Fatalf("expected the deciding node's id, got %q", got.ModeratorID)
	}
	if err := got.Verify(privKey.Public().(ed25519.PublicKey)); err != nil {
		t.Fatalf("the verdict must be signed by the deciding node: %v", err)
	}
	if pub.calls != 1 {
		t.Fatalf("a FAIL verdict must be broadcast to followers once, got %d", pub.calls)
	}
}

func TestDecided_OkNotifiesReporterWithoutIsolation(t *testing.T) {
	m, pub, delivered, _ := decidedFixture(t, nil)

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	m.Decided(rep, decidedBallot(domain.OK, ""), []domain.ID{"mod-a"})

	if len(*delivered) != 1 || (*delivered)[0].Verdict != domain.OK {
		t.Fatalf("the reporter must hear the OK verdict, got %+v", *delivered)
	}
	if pub.calls != 0 {
		t.Fatalf("an OK verdict must not be broadcast to followers, got %d", pub.calls)
	}
}

// An older client sends no reporter identity: nothing to notify, but the
// shadow-ban still goes out.
func TestDecided_NoReporterStillIsolates(t *testing.T) {
	m, pub, delivered, _ := decidedFixture(t, nil)

	m.Decided(tweetReport("tweet-1"), decidedBallot(domain.FAIL, "Hate"), []domain.ID{"mod-a"})

	if len(*delivered) != 0 {
		t.Fatalf("without a reporter node there is nobody to notify, got %+v", *delivered)
	}
	if pub.calls != 1 {
		t.Fatalf("the isolation broadcast must still fire, got %d", pub.calls)
	}
}

func TestDecided_UserVerdictIsolatesProfile(t *testing.T) {
	m, pub, delivered, _ := decidedFixture(t, nil)

	rep := userReport()
	outcome := vote.Event{
		ReportID: "round-1", Type: domain.ModerationUserType,
		Result: domain.FAIL, UserID: "offender",
	}
	m.Decided(rep, outcome, []domain.ID{"mod-a"})

	if len(*delivered) != 1 || (*delivered)[0].Type != domain.ModerationUserType {
		t.Fatalf("the reporter must hear a profile verdict, got %+v", *delivered)
	}
	if pub.calls != 1 {
		t.Fatalf("a FAIL profile verdict must be broadcast, got %d", pub.calls)
	}
}

// --- audit references ---

// A decided round files what this node judged, labelled with the verdict
// the quorum reached — that is where audit references come from.
func TestDecided_FilesTheJudgedTextAsReference(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	node := stubModeratorNode{
		id:   "mod-self",
		resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: "the judged text", UserId: "offender"}),
	}
	m, _ := newTestModerator(t, node, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	if _, ok, err := m.Ballot(rep.ReportID(), rep); err != nil || !ok {
		t.Fatalf("Ballot: %v ok=%t", err, ok)
	}

	m.Decided(rep, decidedBallot(domain.FAIL, "Hate"), []domain.ID{"mod-a"})

	_, unsafe := m.corpus.Len()
	if unsafe != 1 {
		t.Fatalf("the decided round must leave one flagged reference, got %d", unsafe)
	}
}

// A round this node never voted on leaves no reference: it never fetched
// the text, so it has nothing to vouch for.
func TestFileReference_WithoutOwnBallotIsNoop(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	m.fileReference("some-round-nobody-here-voted-on", domain.FAIL)

	if safe, unsafe := m.corpus.Len(); safe != 0 || unsafe != 0 {
		t.Fatalf("expected an empty corpus, got %d/%d", safe, unsafe)
	}
}

// Hearing the Final announcement is enough: every voter files the reference,
// not just the node that carried the decision.
func TestHandleVote_FinalFilesTheReference(t *testing.T) {
	withEngine(t, fixedEngine{ok: true})

	node := stubModeratorNode{
		id:   "mod-self",
		resp: marshal(t, domain.Tweet{Id: "tweet-1", Text: "a clean text", UserId: "author"}),
	}
	m, _ := newTestModerator(t, node, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	if _, ok, err := m.Ballot(rep.ReportID(), rep); err != nil || !ok {
		t.Fatalf("Ballot: %v ok=%t", err, ok)
	}

	final := vote.Event{
		ReportID: rep.ReportID(), Type: domain.ModerationTweetType,
		Result: domain.OK, ModeratorID: "some-chair", Final: true,
	}
	if err := m.handleVote(final); err != nil {
		t.Fatalf("handleVote: %v", err)
	}

	if safe, _ := m.corpus.Len(); safe != 1 {
		t.Fatalf("the announcement must leave one clean reference, got %d", safe)
	}
}

// Unreviewable content judged nothing, so it must never become a reference.
func TestBallot_UnreviewableLeavesNoReference(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{id: "mod-self", resp: []byte(`{}`)}
	m, _ := newTestModerator(t, node, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	if _, ok, err := m.Ballot(rep.ReportID(), rep); err != nil || !ok {
		t.Fatalf("Ballot: %v ok=%t", err, ok)
	}
	m.Decided(rep, decidedBallot(domain.OK, ""), []domain.ID{"mod-a"})

	if safe, unsafe := m.corpus.Len(); safe != 0 || unsafe != 0 {
		t.Fatalf("nothing was judged, so nothing may be referenced: %d/%d", safe, unsafe)
	}
}

func (stubModeratorNode) Rating() *rating.Handle { return rating.NewHandle() }
