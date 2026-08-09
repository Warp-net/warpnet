//nolint:all
package round

import (
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

// stubMember is a Participant that votes a fixed way and records what the
// round asked of it. No moderator, no engine: this package must be testable
// without knowing what a ballot means.
type stubMember struct {
	id     string
	result domain.ModerationResult
	reason string
	// abstain makes Ballot decline to vote.
	abstain bool

	mu        sync.Mutex
	ballots   int
	broadcast []vote.Event
	decisions []decision
	ready     chan struct{}
}

type decision struct {
	subject event.ReportEvent
	outcome vote.Event
	voters  []domain.ID
}

func newStubMember(id string, result domain.ModerationResult) *stubMember {
	return &stubMember{id: id, result: result, reason: "because", ready: make(chan struct{}, 8)}
}

func (s *stubMember) Ballot(reportID string, subject event.ReportEvent) (vote.Event, bool, error) {
	s.mu.Lock()
	s.ballots++
	s.mu.Unlock()
	if s.abstain {
		return vote.Event{}, false, nil
	}
	reason := s.reason
	return vote.Event{
		ReportID:    reportID,
		Type:        subject.Type,
		Result:      s.result,
		Reason:      &reason,
		UserID:      subject.TargetUserID,
		ObjectID:    subject.ObjectID,
		ModeratorID: s.id,
	}, true, nil
}

func (s *stubMember) Broadcast(v vote.Event) error {
	s.mu.Lock()
	s.broadcast = append(s.broadcast, v)
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
	return nil
}

func (s *stubMember) Decided(subject event.ReportEvent, outcome vote.Event, voters []domain.ID) {
	s.mu.Lock()
	s.decisions = append(s.decisions, decision{subject, outcome, voters})
	s.mu.Unlock()
}

func (s *stubMember) counts() (ballots int, broadcasts int, decisions int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ballots, len(s.broadcast), len(s.decisions)
}

func (s *stubMember) lastDecision(t *testing.T) decision {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.decisions) == 0 {
		t.Fatal("the round never handed over a decision")
	}
	return s.decisions[len(s.decisions)-1]
}

func (s *stubMember) finals() []vote.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]vote.Event, 0, len(s.broadcast))
	for _, v := range s.broadcast {
		if v.Final {
			out = append(out, v)
		}
	}
	return out
}

// waitBallot blocks until the round broadcasts something.
func (s *stubMember) waitBallot(t *testing.T) {
	t.Helper()
	select {
	case <-s.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the round never broadcast a ballot")
	}
}

// frozen keeps every timer far away so tests drive the phases by hand.
func frozen() Schedule {
	return Schedule{Window: time.Hour, Failover: time.Hour, Step: time.Hour}
}

func subject(objectID string) event.ReportEvent {
	id := domain.ID(objectID)
	return event.ReportEvent{
		Type:         domain.ModerationTweetType,
		TargetUserID: "offender",
		TargetNodeID: "node-1",
		ObjectID:     &id,
		Reason:       "Hate",
	}
}

func ballotFrom(reportID, member string, result domain.ModerationResult) vote.Event {
	objectID := domain.ID("tweet-1")
	return vote.Event{
		ReportID:    reportID,
		Type:        domain.ModerationTweetType,
		Result:      result,
		UserID:      "offender",
		ObjectID:    &objectID,
		ModeratorID: member,
	}
}

func liveRound(rs *Registry, id string) *round {
	rs.mx.Lock()
	defer rs.mx.Unlock()
	return rs.active[id]
}

func activeCount(rs *Registry) int {
	rs.mx.Lock()
	defer rs.mx.Unlock()
	return len(rs.active)
}

// closeWindow runs the tally as if the vote window had elapsed.
func closeWindow(t *testing.T, rs *Registry, id string) {
	t.Helper()
	r := liveRound(rs, id)
	if r == nil {
		t.Fatalf("no live round for %s", id)
	}
	r.tally()
}

// rankedPeers finds n synthetic participant ids that sort above (or below)
// the pivot for this round, to force a chair outcome.
func rankedPeers(t *testing.T, reportID, pivot string, above bool, n int) []string {
	t.Helper()
	pivotHash := pairHash(reportID, pivot)
	out := make([]string, 0, n)
	for i := 0; i < 10000 && len(out) < n; i++ {
		id := "peer-" + strconv.Itoa(i)
		h := pairHash(reportID, id)
		if (above && h > pivotHash) || (!above && h < pivotHash) {
			out = append(out, id)
		}
	}
	if len(out) < n {
		t.Fatalf("could not find %d peers on the requested side of the pivot", n)
	}
	return out
}

// A lone participant is its own chair: it votes, tallies and carries the
// decision.
func TestRound_SoleVoterDecides(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	rs.Open(subj)
	member.waitBallot(t)

	closeWindow(t, rs, subj.ReportID())

	d := member.lastDecision(t)
	if d.outcome.Result != domain.FAIL {
		t.Fatalf("expected the FAIL ballot to carry, got %+v", d.outcome)
	}
	if len(d.voters) != 1 || d.voters[0] != "self" {
		t.Fatalf("expected a single voter, got %v", d.voters)
	}
	if finals := member.finals(); len(finals) != 1 {
		t.Fatalf("the decision must be announced exactly once, got %d", len(finals))
	}
	if activeCount(rs) != 0 {
		t.Fatal("a decided round must not stay active")
	}
}

// Two FAIL ballots against one OK: strict majority decides.
func TestRound_MajorityDecides(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	others := rankedPeers(t, id, "self", true, 2) // self ranks first -> chair

	rs.Open(subj)
	member.waitBallot(t)
	rs.AddVote(ballotFrom(id, others[0], domain.FAIL))
	rs.AddVote(ballotFrom(id, others[1], domain.OK))

	closeWindow(t, rs, id)

	d := member.lastDecision(t)
	if d.outcome.Result != domain.FAIL {
		t.Fatal("2 of 3 FAIL must decide FAIL")
	}
	if len(d.voters) != 3 {
		t.Fatalf("expected 3 voters, got %v", d.voters)
	}
}

// The same round with the majority the other way: one FAIL cannot outvote
// two OK ballots.
func TestRound_MajorityOverrulesOwnBallot(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	others := rankedPeers(t, id, "self", true, 2)

	rs.Open(subj)
	member.waitBallot(t)
	rs.AddVote(ballotFrom(id, others[0], domain.OK))
	rs.AddVote(ballotFrom(id, others[1], domain.OK))

	closeWindow(t, rs, id)

	if d := member.lastDecision(t); d.outcome.Result != domain.OK {
		t.Fatal("2 of 3 OK must decide OK")
	}
}

// A participant that is not the chair tallies but stays silent.
func TestRound_NonChairStandsBy(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	below := rankedPeers(t, id, "self", false, 1) // outranks self -> chair
	above := rankedPeers(t, id, "self", true, 1)

	rs.Open(subj)
	member.waitBallot(t)
	rs.AddVote(ballotFrom(id, below[0], domain.FAIL))
	rs.AddVote(ballotFrom(id, above[0], domain.FAIL))

	closeWindow(t, rs, id)

	if _, _, decisions := member.counts(); decisions != 0 {
		t.Fatalf("a non-chair must not carry the decision, got %d", decisions)
	}
	if liveRound(rs, id) == nil {
		t.Fatal("a backup must stay parked, ready to take over")
	}
}

// The chair goes silent: the first backup takes the decision over.
func TestRound_BackupTakesOverSilentChair(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	below := rankedPeers(t, id, "self", false, 1)
	above := rankedPeers(t, id, "self", true, 1)

	rs.Open(subj)
	member.waitBallot(t)
	rs.AddVote(ballotFrom(id, below[0], domain.FAIL))
	rs.AddVote(ballotFrom(id, above[0], domain.FAIL))
	closeWindow(t, rs, id)

	r := liveRound(rs, id)
	if r == nil || r.pending == nil || r.finalTimer == nil {
		t.Fatalf("the backup must park the outcome and schedule a takeover, got %+v", r)
	}

	r.takeOver()

	d := member.lastDecision(t)
	if d.outcome.Result != domain.FAIL {
		t.Fatalf("the backup must carry the tallied outcome, got %+v", d.outcome)
	}
	if finals := member.finals(); len(finals) != 1 || finals[0].ModeratorID != "self" {
		t.Fatalf("the backup announces under its own identity, got %+v", finals)
	}
}

// A Final announcement from the chair cancels the parked takeover.
func TestRound_FinalAnnouncementCancelsTakeover(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	below := rankedPeers(t, id, "self", false, 1)
	above := rankedPeers(t, id, "self", true, 1)

	rs.Open(subj)
	member.waitBallot(t)
	rs.AddVote(ballotFrom(id, below[0], domain.FAIL))
	rs.AddVote(ballotFrom(id, above[0], domain.FAIL))
	closeWindow(t, rs, id)

	parked := liveRound(rs, id)
	if parked == nil {
		t.Fatal("the backup must be standing by before the announcement")
	}

	rs.MarkFinalized(id, below[0])
	if activeCount(rs) != 0 {
		t.Fatal("the announcement must clear the parked round")
	}

	parked.takeOver() // the slot fires anyway; must be a no-op
	if _, _, decisions := member.counts(); decisions != 0 {
		t.Fatalf("a cancelled takeover must not decide, got %d", decisions)
	}
}

// A round already served by a quorum costs a late voter nothing: no ballot,
// no broadcast.
func TestRound_LateVoterIsSuppressed(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	for i, peer := range rankedPeers(t, id, "self", true, quorumTarget) {
		result := domain.FAIL
		if i%2 == 0 {
			result = domain.OK
		}
		rs.AddVote(ballotFrom(id, peer, result))
	}

	// The subject arrives after the quorum; the volunteer timer is frozen,
	// so fire this participant's turn by hand.
	rs.Open(subj)
	liveRound(rs, id).castVote()

	if ballots, broadcasts, _ := member.counts(); ballots != 0 || broadcasts != 0 {
		t.Fatalf("a suppressed voter must not be asked for a ballot, got %d ballots %d broadcasts", ballots, broadcasts)
	}
}

// An abstaining participant produces no ballot at all.
func TestRound_AbstainingVoterBroadcastsNothing(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	member.abstain = true
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	rs.Open(subj)
	liveRound(rs, subj.ReportID()).castVote()

	if ballots, broadcasts, _ := member.counts(); ballots != 1 || broadcasts != 0 {
		t.Fatalf("an abstention must not broadcast, got %d ballots %d broadcasts", ballots, broadcasts)
	}
}

func TestRegistry_DedupsBallotsByParticipant(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	id := subject("tweet-1").ReportID()
	rs.AddVote(ballotFrom(id, "peer-a", domain.FAIL))
	rs.AddVote(ballotFrom(id, "peer-a", domain.OK)) // second ballot, same peer

	r := liveRound(rs, id)
	r.mx.Lock()
	defer r.mx.Unlock()
	if len(r.votes) != 1 {
		t.Fatalf("expected exactly one counted ballot, got %d", len(r.votes))
	}
	if bool(r.votes["peer-a"].Result) {
		t.Fatal("the first ballot must win, not be overwritten")
	}
}

func TestRegistry_DecidedRoundIsNotReopened(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	subj := subject("tweet-1")
	id := subj.ReportID()
	rs.AddVote(ballotFrom(id, "peer-a", domain.FAIL))
	closeWindow(t, rs, id)

	// Late gossip after the round is spent.
	rs.AddVote(ballotFrom(id, "peer-b", domain.FAIL))
	rs.Open(subj)

	if activeCount(rs) != 0 {
		t.Fatal("a spent round must not be reopened by late gossip")
	}
}

func TestRegistry_FinalAnnouncementIsNotABallot(t *testing.T) {
	member := newStubMember("self", domain.FAIL)
	rs := NewRegistry("self", member, frozen())
	t.Cleanup(rs.StopAll)

	id := subject("tweet-1").ReportID()
	rs.MarkFinalized(id, "peer-a")

	if activeCount(rs) != 0 {
		t.Fatal("an announcement must not open a round")
	}
	rs.AddVote(ballotFrom(id, "peer-b", domain.FAIL))
	if activeCount(rs) != 0 {
		t.Fatal("late ballots must not reopen a decided round")
	}
}

func TestVoteDelay_SmallPopulationStartsAtOnce(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), DefaultSchedule())

	rs.mx.Lock()
	defer rs.mx.Unlock()
	for _, id := range []string{"round-1", "round-2", "round-3", "round-4"} {
		if d := rs.voteDelayLocked(id); d != 0 {
			t.Fatalf("a lone participant must start at once, got %v for %s", d, id)
		}
	}
	rs.seenMods["peer-a"] = time.Now()
	rs.seenMods["peer-b"] = time.Now()
	for _, id := range []string{"round-1", "round-2", "round-3", "round-4"} {
		if d := rs.voteDelayLocked(id); d != 0 {
			t.Fatalf("a population of %d must start at once, got %v for %s", quorumTarget, d, id)
		}
	}
}

func TestVoteDelay_LargePopulationDefersMostRounds(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), DefaultSchedule())

	rs.mx.Lock()
	defer rs.mx.Unlock()
	for i := 0; i < 200; i++ {
		rs.seenMods["peer-"+strconv.Itoa(i)] = time.Now()
	}
	deferred := 0
	for i := 0; i < 20; i++ {
		d := rs.voteDelayLocked("round-" + strconv.Itoa(i))
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
	// With ~200 participants the odds of ranking in the top 3 are ~1.5%
	// per round; 20 rounds all landing there is impossible in practice.
	if deferred == 0 {
		t.Fatal("with 200 participants most rounds must be deferred")
	}
}

func TestKeptVotes_EvenCountTrimmedDeterministically(t *testing.T) {
	id := "round-x"
	ballots := map[string]vote.Event{}
	for _, peer := range []string{"peer-a", "peer-b", "peer-c", "peer-d"} {
		ballots[peer] = vote.Event{ReportID: id, ModeratorID: peer}
	}

	kept := keptVotes(id, ballots)
	if len(kept) != 3 {
		t.Fatalf("4 ballots must be trimmed to 3, got %d", len(kept))
	}

	order := []string{"peer-a", "peer-b", "peer-c", "peer-d"}
	sort.Slice(order, func(i, j int) bool { return pairHash(id, order[i]) < pairHash(id, order[j]) })
	for i := range kept {
		if kept[i].ModeratorID != order[i] {
			t.Fatalf("kept ballots must be hash-ordered: got %s at %d, want %s", kept[i].ModeratorID, i, order[i])
		}
	}
	dropped := order[len(order)-1]
	for _, v := range kept {
		if v.ModeratorID == dropped {
			t.Fatalf("the highest-ranked ballot %s must be the one dropped", dropped)
		}
	}
}

func TestAggregate_MajorityAndTieRules(t *testing.T) {
	mk := func(id string, res domain.ModerationResult) vote.Event {
		return ballotFrom("round-x", id, res)
	}

	outcome, voters := aggregate([]vote.Event{
		mk("peer-a", domain.FAIL), mk("peer-b", domain.OK), mk("peer-c", domain.FAIL),
	})
	if outcome.Result != domain.FAIL {
		t.Fatal("2 of 3 FAIL must aggregate to FAIL")
	}
	if len(voters) != 3 {
		t.Fatalf("expected 3 voters, got %v", voters)
	}

	outcome, _ = aggregate([]vote.Event{
		mk("peer-a", domain.FAIL), mk("peer-b", domain.OK), mk("peer-c", domain.OK),
	})
	if outcome.Result != domain.OK {
		t.Fatal("1 of 3 FAIL must aggregate to OK")
	}

	// A tie is defensive only (keptVotes prevents it) and must not condemn.
	outcome, _ = aggregate([]vote.Event{mk("peer-a", domain.FAIL), mk("peer-b", domain.OK)})
	if outcome.Result != domain.OK {
		t.Fatal("a tie must not decide FAIL")
	}

	outcome, voters = aggregate([]vote.Event{mk("peer-a", domain.FAIL)})
	if outcome.Result != domain.FAIL || len(voters) != 1 {
		t.Fatalf("a single FAIL ballot must stand, got %+v %v", outcome, voters)
	}
}

// planTally is the whole rule of a round in one pure function, so it is
// testable without a registry, a timer or a participant.
func TestPlanTally_Roles(t *testing.T) {
	id := "round-x"
	ballots := func(ids ...string) map[string]vote.Event {
		out := map[string]vote.Event{}
		for _, peer := range ids {
			out[peer] = ballotFrom(id, peer, domain.FAIL)
		}
		return out
	}
	byRank := func(ids ...string) []string {
		out := append([]string(nil), ids...)
		sort.Slice(out, func(i, j int) bool { return pairHash(id, out[i]) < pairHash(id, out[j]) })
		return out
	}

	order := byRank("peer-a", "peer-b", "peer-c")

	p := planTally(id, order[0], ballots(order...), true)
	if p.role != roleChair || p.rank != 0 || p.chair != order[0] {
		t.Fatalf("the lowest-ranked voter chairs, got %+v", p)
	}
	if p.counted != 3 || len(p.voters) != 3 {
		t.Fatalf("all three ballots must count, got %+v", p)
	}

	p = planTally(id, order[1], ballots(order...), true)
	if p.role != roleBackup || p.rank != 1 {
		t.Fatalf("the next voter stands by at rank 1, got %+v", p)
	}

	p = planTally(id, "never-voted", ballots(order...), true)
	if p.role != roleBystander {
		t.Fatalf("a non-voter has nothing to do, got %+v", p)
	}

	p = planTally(id, order[0], map[string]vote.Event{}, true)
	if p.role != roleBystander {
		t.Fatalf("an empty round decides nothing, got %+v", p)
	}

	// Ballots naming this participant on a subject it never saw: forged.
	p = planTally(id, order[0], ballots(order...), false)
	if p.role != roleBystander || !p.orphaned {
		t.Fatalf("a voter without the subject must stand down as orphaned, got %+v", p)
	}
}

// The trimmed-away voter keeps its place in the takeover chain, so a
// two-ballot round still has a backup behind the chair.
func TestPlanTally_TrimmedVoterStaysInTheChain(t *testing.T) {
	id := "round-y"
	order := []string{"peer-a", "peer-b"}
	sort.Slice(order, func(i, j int) bool { return pairHash(id, order[i]) < pairHash(id, order[j]) })

	ballots := map[string]vote.Event{
		order[0]: ballotFrom(id, order[0], domain.FAIL),
		order[1]: ballotFrom(id, order[1], domain.FAIL),
	}

	p := planTally(id, order[1], ballots, true)
	if p.role != roleBackup || p.rank != 1 {
		t.Fatalf("the trimmed voter must still guard the round, got %+v", p)
	}
	if p.counted != 1 {
		t.Fatalf("the tally itself keeps an odd count, got %d", p.counted)
	}
}
