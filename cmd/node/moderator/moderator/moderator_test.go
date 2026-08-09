//nolint:all
package moderator

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

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
	published chan event.ModerationVoteEvent
}

func (s *stubVotes) PublishVote(ev event.ModerationVoteEvent) error {
	if s.published != nil {
		s.published <- ev
	}
	return nil
}

func (s *stubVotes) SubscribeVotes(func(ev event.ModerationVoteEvent) error) error { return nil }

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
	// Not zero: the retrier's jitter draws from minInterval/2 and panics
	// on an empty range.
	fetchRetryDelay = time.Millisecond
	t.Cleanup(func() { fetchRetryDelay = prev })
}

func fastRetrier() retrier.Retrier {
	return retrier.New(time.Millisecond, fetchAttempts, retrier.FixedBackoff)
}

// newTestModerator builds a full moderator whose round timers are pushed out
// to an hour, so tests drive the round phases (vote, tally, takeover)
// deterministically. The volunteer timer still fires immediately, since a
// single-moderator population ranks 0.
func newTestModerator(t *testing.T, node ModeratorNode, pub Publisher, privKey ed25519.PrivateKey) (*Moderator, *stubVotes) {
	t.Helper()
	votes := &stubVotes{published: make(chan event.ModerationVoteEvent, 8)}
	m, err := NewModerator(context.Background(), node, pub, nil, votes, privKey)
	if err != nil {
		t.Fatalf("NewModerator: %v", err)
	}
	prevWindow, prevStep, prevFailover := voteWindow, voteDelayStep, failoverDelay
	voteWindow, voteDelayStep, failoverDelay = time.Hour, time.Hour, time.Hour
	t.Cleanup(func() {
		voteWindow, voteDelayStep, failoverDelay = prevWindow, prevStep, prevFailover
	})
	return m, votes
}

// roundOf returns the live round, or nil when the moderator no longer holds
// one for that report.
func roundOf(m *Moderator, id string) *round {
	m.rounds.mx.Lock()
	defer m.rounds.mx.Unlock()
	return m.rounds.active[id]
}

// isSpent reports whether the moderator remembers the round as finished.
func isSpent(m *Moderator, id string) bool {
	m.rounds.mx.Lock()
	defer m.rounds.mx.Unlock()
	_, ok := m.rounds.finalized[id]
	return ok
}

func activeRounds(m *Moderator) int {
	m.rounds.mx.Lock()
	defer m.rounds.mx.Unlock()
	return len(m.rounds.active)
}

// closeWindow runs the tally as if the vote window had elapsed.
func closeWindow(t *testing.T, m *Moderator, id string) {
	t.Helper()
	r := roundOf(m, id)
	if r == nil {
		t.Fatalf("no live round for %s", id)
	}
	r.tally()
}

func votesOf(m *Moderator, id string) map[string]event.ModerationVoteEvent {
	r := roundOf(m, id)
	if r == nil {
		return nil
	}
	r.mx.Lock()
	defer r.mx.Unlock()
	out := make(map[string]event.ModerationVoteEvent, len(r.votes))
	for k, v := range r.votes {
		out[k] = v
	}
	return out
}

func waitVote(t *testing.T, votes *stubVotes) event.ModerationVoteEvent {
	t.Helper()
	select {
	case v := <-votes.published:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("vote was never published")
		return event.ModerationVoteEvent{}
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

func assertUnavailable(t *testing.T, v verdict) {
	t.Helper()
	if v.result != domain.OK {
		t.Fatal("an unreviewable report must carry an OK (no-op) result")
	}
	if v.reason == nil || *v.reason != event.ModerationReasonUnavailable {
		t.Fatalf("expected the unavailable sentinel reason, got %v", v.reason)
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
	if v.result != domain.OK {
		t.Fatal("recordingEngine clears content, expected an OK verdict")
	}
	if v.objectID == nil || *v.objectID != "tweet-1" || v.userID != "offender" {
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
	if v.result != domain.FAIL {
		t.Fatal("the stored FAIL verdict must be reused")
	}
	if v.reason == nil || *v.reason != reason {
		t.Fatalf("the stored reason must be reused, got %v", v.reason)
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
	if v.result != domain.OK || v.userID != "user-1" {
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

func TestHandleReport_ClosedModeratorIsNoop(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			t.Fatal("a closed moderator must not dial anything")
			return nil, nil
		},
	}
	m, _ := newTestModerator(t, node, stubPublisher{}, nil)
	m.isClosed.Store(true)

	if err := m.handleReport(tweetReport("tweet-1")); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	if activeRounds(m) != 0 {
		t.Fatal("a closed moderator must not open rounds")
	}
	if rec.called {
		t.Fatal("engine must not run on a closed moderator")
	}
}

func TestHandleReport_MissingObjectIDOpensNoRound(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{}, stubPublisher{}, nil)

	rep := tweetReport("")
	rep.ObjectID = nil
	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	if activeRounds(m) != 0 {
		t.Fatal("a malformed report must not open a round")
	}
}

// chairCandidates returns the given moderator ids ordered by their pair hash
// for the report, i.e. index 0 is the chair if all of them vote.
func chairCandidates(reportID string, ids ...string) []string {
	sorted := append([]string(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool {
		return pairHash(reportID, sorted[i]) < pairHash(reportID, sorted[j])
	})
	return sorted
}

// selfVoterID is the identity a stub node writes into its own votes: peer.ID
// base58-encodes its raw bytes, so it differs from the raw stub id string.
func selfVoterID(raw string) string { return warpnet.WarpPeerID(raw).String() }

// pickPeers finds n synthetic moderator ids whose pair hash for the round is
// above (or below) the pivot voter's, to force chair outcomes in tests.
func pickPeers(t *testing.T, reportID, pivot string, above bool, n int) []string {
	t.Helper()
	pivotHash := pairHash(reportID, pivot)
	out := make([]string, 0, n)
	for i := 0; i < 10000 && len(out) < n; i++ {
		id := "mod-" + strconv.Itoa(i)
		h := pairHash(reportID, id)
		if (above && h > pivotHash) || (!above && h < pivotHash) {
			out = append(out, id)
		}
	}
	if len(out) < n {
		t.Fatalf("could not find %d peers %s the pivot", n, map[bool]string{true: "above", false: "below"}[above])
	}
	return out
}

func otherVote(reportID string, moderatorID string, result domain.ModerationResult, reason string) event.ModerationVoteEvent {
	objectID := domain.ID("tweet-1")
	return event.ModerationVoteEvent{
		ReportID:    reportID,
		Type:        domain.ModerationTweetType,
		Result:      result,
		Reason:      &reason,
		UserID:      "offender",
		ObjectID:    &objectID,
		ModeratorID: moderatorID,
	}
}

// Full single-moderator round: the only voter is its own chair, so the FAIL
// verdict reaches the reporter and the isolation broadcast fires — the
// pre-trio behavior, minus the wait.
func TestRound_SingleModeratorFailFinalizes(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	var (
		delivered int
		gotNode   string
		gotResult event.ModerationVerdictEvent
	)
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				gotNode = nodeId
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	vote := waitVote(t, votes)
	if vote.Result != domain.FAIL || vote.ReportID != rep.ReportID() {
		t.Fatalf("unexpected vote %+v", vote)
	}

	closeWindow(t, m, rep.ReportID())

	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotNode != "reporter-node-1" {
		t.Fatalf("expected delivery to the reporter node, got %q", gotNode)
	}
	if gotResult.ReporterID != "reporter-1" {
		t.Fatalf("expected reporter id propagated, got %q", gotResult.ReporterID)
	}
	if gotResult.Verdict != domain.FAIL {
		t.Fatal("expected the FAIL verdict to be propagated to the reporter")
	}
	if len(gotResult.Voters) != 1 || gotResult.Voters[0] != selfVoterID("mod-self") {
		t.Fatalf("expected the voter list [%s], got %v", selfVoterID("mod-self"), gotResult.Voters)
	}
	if pub.calls != 1 {
		t.Fatalf("a FAIL verdict must be broadcast to followers exactly once, got %d", pub.calls)
	}
}

func TestRound_SingleModeratorOkVerdictSkipsIsolation(t *testing.T) {
	withEngine(t, fixedEngine{ok: true})

	var (
		delivered int
		gotResult event.ModerationVerdictEvent
	)
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	closeWindow(t, m, rep.ReportID())

	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Verdict != domain.OK {
		t.Fatal("expected the OK verdict to be propagated to the reporter")
	}
	if pub.calls != 0 {
		t.Fatalf("an OK verdict must not be broadcast to followers, got %d publishes", pub.calls)
	}
}

// No reporter identity (older client) -> moderated, but no reporter delivery.
func TestRound_NoReporterNoDelivery(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				t.Fatal("must not deliver to a reporter without a reporter node id")
			}
			return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	rep := tweetReport("tweet-1")
	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	closeWindow(t, m, rep.ReportID())

	if pub.calls != 1 {
		t.Fatalf("the isolation broadcast must still fire, got %d publishes", pub.calls)
	}
}

// Trio round: self is the chair, votes FAIL along with one other FAIL and
// one OK — the strict majority isolates and the reporter hears FAIL.
func TestRound_MajorityFailWins(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	// Both other voters rank above self, making self the chair.
	others := pickPeers(t, reportID, selfVoterID("mod-self"), true, 2)

	var (
		delivered int
		gotResult event.ModerationVerdictEvent
	)
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	if err := m.handleVote(otherVote(reportID, others[0], domain.FAIL, "Hate")); err != nil {
		t.Fatalf("handleVote: %v", err)
	}
	if err := m.handleVote(otherVote(reportID, others[1], domain.OK, "")); err != nil {
		t.Fatalf("handleVote: %v", err)
	}

	closeWindow(t, m, reportID)

	if delivered != 1 {
		t.Fatalf("expected exactly one reporter delivery, got %d", delivered)
	}
	if gotResult.Verdict != domain.FAIL {
		t.Fatal("2 of 3 FAIL votes must produce a FAIL verdict")
	}
	if len(gotResult.Voters) != 3 {
		t.Fatalf("expected 3 voters in the result, got %v", gotResult.Voters)
	}
	if pub.calls != 1 {
		t.Fatalf("the majority FAIL must isolate exactly once, got %d", pub.calls)
	}
}

// Trio round with self as chair, but the majority clears the content: one
// FAIL (self) against two OK — no isolation, reporter hears OK.
func TestRound_MajorityOkOverrulesOwnFail(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	others := pickPeers(t, reportID, selfVoterID("mod-self"), true, 2)

	var gotResult event.ModerationVerdictEvent
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	_ = m.handleVote(otherVote(reportID, others[0], domain.OK, ""))
	_ = m.handleVote(otherVote(reportID, others[1], domain.OK, ""))

	closeWindow(t, m, reportID)

	if gotResult.Verdict != domain.OK {
		t.Fatal("2 of 3 OK votes must clear the content")
	}
	if pub.calls != 0 {
		t.Fatalf("a majority OK must not isolate, got %d publishes", pub.calls)
	}
}

// A moderator that is not the round's chair tallies but stays silent.
func TestRound_NonChairDoesNotFinalize(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	// One voter ranks below self (the chair), one above.
	below := pickPeers(t, reportID, selfVoterID("mod-self"), false, 1)
	above := pickPeers(t, reportID, selfVoterID("mod-self"), true, 1)

	node := stubModeratorNode{
		id: "mod-self", // not the chair
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				t.Fatal("a non-chair moderator must not notify the reporter")
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	_ = m.handleVote(otherVote(reportID, below[0], domain.FAIL, "Hate"))
	_ = m.handleVote(otherVote(reportID, above[0], domain.FAIL, "Hate"))

	closeWindow(t, m, reportID)

	if pub.calls != 0 {
		t.Fatalf("a non-chair moderator must not isolate, got %d publishes", pub.calls)
	}
}

// The volunteer timer of a low-ranked moderator finds the round already
// served and must not fetch or run inference at all.
func TestCastVote_SuppressedWhenQuorumReached(t *testing.T) {
	rec := &recordingEngine{}
	withEngine(t, rec)

	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, _ stream.WarpRoute, _ any) ([]byte, error) {
			t.Fatal("a suppressed moderator must not dial anything")
			return nil, nil
		},
	}
	m, votes := newTestModerator(t, node, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	reportID := rep.ReportID()
	_ = m.handleVote(otherVote(reportID, "mod-a", domain.FAIL, "Hate"))
	_ = m.handleVote(otherVote(reportID, "mod-b", domain.OK, ""))
	_ = m.handleVote(otherVote(reportID, "mod-c", domain.OK, ""))

	// Hand the round its report without arming the volunteer timer, then
	// fire the vote step by hand.
	roundOf(m, reportID).setReport(rep, time.Hour)
	roundOf(m, reportID).castVote()

	select {
	case v := <-votes.published:
		t.Fatalf("a suppressed moderator must not vote, published %+v", v)
	default:
	}
	if rec.called {
		t.Fatal("a suppressed moderator must not run the engine")
	}
}

func TestHandleVote_DedupsByModerator(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	reportID := rep.ReportID()
	_ = m.handleVote(otherVote(reportID, "mod-a", domain.FAIL, "Hate"))
	_ = m.handleVote(otherVote(reportID, "mod-a", domain.OK, "")) // second vote, same moderator

	counted := votesOf(m, reportID)
	if len(counted) != 1 {
		t.Fatalf("expected exactly one counted vote, got %+v", counted)
	}
	if bool(counted["mod-a"].Result) {
		t.Fatal("the first vote must win, not be overwritten")
	}
}

func TestClosedRound_IsNotReopened(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	reportID := rep.ReportID()
	_ = m.handleVote(otherVote(reportID, "mod-a", domain.FAIL, "Hate"))
	closeWindow(t, m, reportID)

	// Gossip re-delivery after the round is finalized.
	_ = m.handleVote(otherVote(reportID, "mod-b", domain.FAIL, "Hate"))
	m.rounds.open(rep)

	if activeRounds(m) != 0 {
		t.Fatal("a finalized round must not be reopened by late gossip")
	}
}

// The chair signs the aggregate verdict with its node key; the signature
// must verify over the event's canonical signing bytes.
func TestRound_ChairSignsResult(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	privKey, err := security.GenerateKeyFromSeed([]byte("moderator-sign-test"))
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	var gotResult event.ModerationVerdictEvent
	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	m, votes := newTestModerator(t, node, stubPublisher{}, privKey)

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	closeWindow(t, m, rep.ReportID())

	if gotResult.Signature == "" {
		t.Fatal("the chair must sign the verdict")
	}
	if gotResult.TimeAt.IsZero() {
		t.Fatal("the signature must cover a real timestamp")
	}
	pubKey := privKey.Public().(ed25519.PublicKey)
	if err := gotResult.Verify(pubKey); err != nil {
		t.Fatalf("the verdict must verify against the chair's key: %v", err)
	}
}

// The chair follows its finalization with a Final announcement on the votes
// topic so backup voters cancel their takeover timers.
func TestRound_ChairAnnouncesFinal(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			return []byte(event.Accepted), nil
		},
	}
	m, votes := newTestModerator(t, node, &recordingPublisher{}, nil)

	rep := tweetReport("tweet-1")
	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	vote := waitVote(t, votes)
	if vote.Final {
		t.Fatal("the first published message must be the vote itself")
	}
	closeWindow(t, m, rep.ReportID())

	final := waitVote(t, votes)
	if !final.Final {
		t.Fatalf("the chair must announce the finalization, got %+v", final)
	}
	if final.ReportID != rep.ReportID() || final.Result != domain.FAIL {
		t.Fatalf("the announcement must carry the aggregate outcome, got %+v", final)
	}
}

// A backup voter parks the tallied outcome at closeRound and finalizes it
// itself once its takeover slot fires with the chair still silent.
func TestRound_BackupTakesOverSilentChair(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	below := pickPeers(t, reportID, selfVoterID("mod-self"), false, 1) // the silent chair
	above := pickPeers(t, reportID, selfVoterID("mod-self"), true, 1)  // keeps the count odd

	var (
		delivered int
		gotResult event.ModerationVerdictEvent
	)
	node := stubModeratorNode{
		id: "mod-self", // rank 1: first backup
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					gotResult = r
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	_ = m.handleVote(otherVote(reportID, below[0], domain.FAIL, "Hate"))
	_ = m.handleVote(otherVote(reportID, above[0], domain.FAIL, "Hate"))

	closeWindow(t, m, reportID)
	if delivered != 0 || pub.calls != 0 {
		t.Fatal("a backup must not finalize while the chair's slot is still open")
	}
	if r := roundOf(m, reportID); r == nil || r.pending == nil || r.finalTimer == nil {
		t.Fatalf("the backup must park the outcome and schedule a takeover, got %+v", r)
	}

	roundOf(m, reportID).takeOver() // the slot fires, chair still silent

	if delivered != 1 {
		t.Fatalf("the backup must notify the reporter, got %d deliveries", delivered)
	}
	if gotResult.Verdict != domain.FAIL || gotResult.ModeratorID != selfVoterID("mod-self") {
		t.Fatalf("the backup finalizes under its own identity, got %+v", gotResult)
	}
	if pub.calls != 1 {
		t.Fatalf("the backup must isolate exactly once, got %d", pub.calls)
	}
	final := waitVote(t, votes)
	if !final.Final {
		t.Fatalf("the backup must announce the finalization, got %+v", final)
	}
}

// A voter dropped by the odd-count trim is still part of the takeover
// chain: on a two-voter round it guards the chair's tally.
func TestRound_TrimmedVoterStillGuards(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	below := pickPeers(t, reportID, selfVoterID("mod-self"), false, 1) // the silent chair

	var delivered int
	node := stubModeratorNode{
		id: "mod-self", // trimmed by the even count, rank 1 in the chain
		streamFn: func(_ string, path stream.WarpRoute, data any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				delivered++
				if r, ok := data.(event.ModerationVerdictEvent); ok {
					if len(r.Voters) != 1 || r.Voters[0] != below[0] {
						t.Fatalf("the tally must stay on the kept set, got voters %v", r.Voters)
					}
				}
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	_ = m.handleVote(otherVote(reportID, below[0], domain.FAIL, "Hate"))

	closeWindow(t, m, reportID)
	r := roundOf(m, reportID)
	if r == nil || r.pending == nil {
		t.Fatal("the trimmed voter must still park the outcome")
	}

	r.takeOver()
	if delivered != 1 || pub.calls != 1 {
		t.Fatalf("the trimmed voter must finalize the kept tally, got %d deliveries %d isolations", delivered, pub.calls)
	}
}

// The chair's Final announcement cancels the backup's parked takeover.
func TestRound_FinalAnnouncementCancelsTakeover(t *testing.T) {
	withEngine(t, fixedEngine{ok: false, reason: "Hate"})

	rep := tweetReport("tweet-1")
	rep.ReporterID = "reporter-1"
	rep.ReporterNodeID = "reporter-node-1"
	reportID := rep.ReportID()
	below := pickPeers(t, reportID, selfVoterID("mod-self"), false, 1)
	above := pickPeers(t, reportID, selfVoterID("mod-self"), true, 1)

	node := stubModeratorNode{
		id: "mod-self",
		streamFn: func(_ string, path stream.WarpRoute, _ any) ([]byte, error) {
			if path == event.PUBLIC_GET_TWEET {
				return marshal(t, domain.Tweet{Id: "tweet-1", Text: "hello world", UserId: "offender"}), nil
			}
			if path == event.PUBLIC_POST_MODERATION_RESULT {
				t.Fatal("the takeover was cancelled, nothing may be delivered")
			}
			return []byte(event.Accepted), nil
		},
	}
	pub := &recordingPublisher{}
	m, votes := newTestModerator(t, node, pub, nil)

	if err := m.handleReport(rep); err != nil {
		t.Fatalf("handleReport: %v", err)
	}
	waitVote(t, votes)
	_ = m.handleVote(otherVote(reportID, below[0], domain.FAIL, "Hate"))
	_ = m.handleVote(otherVote(reportID, above[0], domain.FAIL, "Hate"))
	closeWindow(t, m, reportID)
	parked := roundOf(m, reportID)
	if parked == nil {
		t.Fatal("the backup must be standing by before the announcement")
	}

	// The chair's announcement arrives before the takeover slot.
	chairFinal := otherVote(reportID, below[0], domain.FAIL, "Hate")
	chairFinal.Final = true
	_ = m.handleVote(chairFinal)

	if activeRounds(m) != 0 {
		t.Fatal("the Final announcement must clear the parked round")
	}

	parked.takeOver() // the slot fires anyway; must be a no-op
	if pub.calls != 0 {
		t.Fatalf("a cancelled takeover must not isolate, got %d", pub.calls)
	}
}

// A Final announcement is control traffic: it must never count as a vote and
// it spends the round for late votes.
func TestHandleVote_FinalIsNotCountedAsVote(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	rep := tweetReport("tweet-1")
	reportID := rep.ReportID()
	final := otherVote(reportID, "mod-a", domain.FAIL, "Hate")
	final.Final = true
	_ = m.handleVote(final)

	if activeRounds(m) != 0 {
		t.Fatal("an announcement must not open a round")
	}
	if !isSpent(m, reportID) {
		t.Fatal("an announcement must mark the round finalized")
	}

	// A late vote after the announcement is ignored.
	_ = m.handleVote(otherVote(reportID, "mod-b", domain.FAIL, "Hate"))
	if activeRounds(m) != 0 {
		t.Fatal("late votes must not reopen a finalized round")
	}
}

func TestVoteDelay_TinyPopulationStartsAtOnce(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	// With population <= quorumTarget every rank lands below the target.
	rs := m.rounds
	rs.mx.Lock()
	defer rs.mx.Unlock()
	for _, id := range []string{"round-1", "round-2", "round-3", "round-4"} {
		if d := rs.voteDelayLocked(id); d != 0 {
			t.Fatalf("single moderator must start at once, got %v for %s", d, id)
		}
	}
	rs.seenMods["mod-a"] = time.Now()
	rs.seenMods["mod-b"] = time.Now()
	for _, id := range []string{"round-1", "round-2", "round-3", "round-4"} {
		if d := rs.voteDelayLocked(id); d != 0 {
			t.Fatalf("population of 3 must start at once, got %v for %s", d, id)
		}
	}
}

func TestVoteDelay_LargePopulationDefersMostRounds(t *testing.T) {
	m, _ := newTestModerator(t, stubModeratorNode{id: "mod-self"}, stubPublisher{}, nil)

	rs := m.rounds
	rs.mx.Lock()
	defer rs.mx.Unlock()
	for i := 0; i < 200; i++ {
		rs.seenMods["mod-"+string(rune('a'+i%26))+string(rune('0'+i/26))] = time.Now()
	}
	deferred := 0
	for i := 0; i < 20; i++ {
		d := rs.voteDelayLocked("round-" + string(rune('a'+i)))
		if d < 0 {
			t.Fatalf("negative delay %v", d)
		}
		if d%voteDelayStep != 0 {
			t.Fatalf("delay must be a whole number of steps, got %v", d)
		}
		if d > 0 {
			deferred++
		}
	}
	// With ~200 moderators the odds of ranking in the top 3 are ~1.5% per
	// round; 20 rounds all landing in the top 3 is impossible in practice.
	if deferred == 0 {
		t.Fatal("with 200 moderators most rounds must be deferred")
	}
}

func TestKeptVotes_EvenCountTrimmedDeterministically(t *testing.T) {
	reportID := "round-x"
	votes := map[string]event.ModerationVoteEvent{}
	for _, id := range []string{"mod-a", "mod-b", "mod-c", "mod-d"} {
		votes[id] = event.ModerationVoteEvent{ReportID: reportID, ModeratorID: id}
	}

	kept := keptVotes(reportID, votes)
	if len(kept) != 3 {
		t.Fatalf("4 votes must be trimmed to 3, got %d", len(kept))
	}
	order := chairCandidates(reportID, "mod-a", "mod-b", "mod-c", "mod-d")
	dropped := order[len(order)-1]
	for _, v := range kept {
		if v.ModeratorID == dropped {
			t.Fatalf("the highest-ranked vote %s must be the one dropped", dropped)
		}
	}
	for i := range kept {
		if kept[i].ModeratorID != order[i] {
			t.Fatalf("kept votes must be hash-ordered: got %s at %d, want %s", kept[i].ModeratorID, i, order[i])
		}
	}
}

func TestAggregate_MajorityAndTieRules(t *testing.T) {
	reason := "Hate"
	mkVote := func(id string, res domain.ModerationResult) event.ModerationVoteEvent {
		objectID := domain.ID("tweet-1")
		return event.ModerationVoteEvent{
			ReportID: "round-x", ModeratorID: id, Result: res,
			Reason: &reason, UserID: "offender", ObjectID: &objectID,
		}
	}

	// 2 FAIL / 1 OK -> FAIL
	agg, voters := aggregate([]event.ModerationVoteEvent{
		mkVote("mod-a", domain.FAIL), mkVote("mod-b", domain.OK), mkVote("mod-c", domain.FAIL),
	})
	if agg.result != domain.FAIL {
		t.Fatal("2 of 3 FAIL must aggregate to FAIL")
	}
	if len(voters) != 3 {
		t.Fatalf("expected 3 voters, got %v", voters)
	}
	if agg.reason == nil || *agg.reason != reason {
		t.Fatalf("the majority reason must be carried, got %v", agg.reason)
	}

	// 1 FAIL / 2 OK -> OK
	agg, _ = aggregate([]event.ModerationVoteEvent{
		mkVote("mod-a", domain.FAIL), mkVote("mod-b", domain.OK), mkVote("mod-c", domain.OK),
	})
	if agg.result != domain.OK {
		t.Fatal("1 of 3 FAIL must aggregate to OK")
	}

	// 1-1 tie (defensive; keptVotes prevents it) -> OK, presumption of innocence
	agg, _ = aggregate([]event.ModerationVoteEvent{
		mkVote("mod-a", domain.FAIL), mkVote("mod-b", domain.OK),
	})
	if agg.result != domain.OK {
		t.Fatal("a tie must not FAIL anyone")
	}

	// single vote -> that vote
	agg, voters = aggregate([]event.ModerationVoteEvent{mkVote("mod-a", domain.FAIL)})
	if agg.result != domain.FAIL || len(voters) != 1 {
		t.Fatalf("a single FAIL vote must stand, got %+v %v", agg, voters)
	}
}
