//nolint:all
package round

import (
	"errors"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/vote"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/stretchr/testify/require"
)

func TestRegistryPeers(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())

	require.Empty(t, rs.Peers(), "nobody has been seen yet")

	rs.AddVote(vote.Event{ReportID: "report-1", ModeratorID: "moderator-a"})
	rs.AddVote(vote.Event{ReportID: "report-1", ModeratorID: "moderator-b"})
	require.ElementsMatch(t, []string{"moderator-a", "moderator-b"}, rs.Peers())

	// a participant last seen beyond the TTL is forgotten on the next read
	rs.mx.Lock()
	rs.seenMods["moderator-a"] = time.Now().Add(-2 * seenModTTL)
	rs.mx.Unlock()

	require.Equal(t, []string{"moderator-b"}, rs.Peers())
}

func TestRegistryLen(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())
	require.Zero(t, rs.Len())

	rs.Open(subject("tweet-1"))
	require.Equal(t, 1, rs.Len())

	rs.StopAll()
	require.Zero(t, rs.Len())
}

func TestMarkFinalizedIsIdempotent(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())
	rs.Open(subject("tweet-1"))
	id := subject("tweet-1").ReportID()

	rs.MarkFinalized(id, "moderator-a")
	require.Zero(t, rs.Len(), "finalizing stops the live round")

	// a second announcement for the same round is dropped rather than
	// re-registering it
	rs.MarkFinalized(id, "moderator-b")
	require.Zero(t, rs.Len())
	require.Contains(t, rs.Peers(), "moderator-b", "the announcer is still seen")
}

func TestFinalizedRoundsExpire(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())
	id := subject("tweet-1").ReportID()

	rs.MarkFinalized(id, "moderator-a")

	rs.mx.Lock()
	require.True(t, rs.isFinalizedLocked(id))
	rs.finalized[id] = time.Now().Add(-2 * finalizedTTL)
	require.False(t, rs.isFinalizedLocked(id), "an expired finalization no longer blocks the round")
	_, still := rs.finalized[id]
	require.False(t, still)
	rs.mx.Unlock()
}

func TestForgetPrunesExpiredFinalizations(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())

	rs.mx.Lock()
	rs.finalized["stale"] = time.Now().Add(-2 * finalizedTTL)
	rs.finalized["fresh"] = time.Now()
	rs.mx.Unlock()

	rs.forget("some-round")

	rs.mx.Lock()
	defer rs.mx.Unlock()
	_, stale := rs.finalized["stale"]
	_, fresh := rs.finalized["fresh"]
	require.False(t, stale)
	require.True(t, fresh)
}

func TestVoteDelayPrunesStaleParticipants(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), DefaultSchedule())

	rs.mx.Lock()
	rs.seenMods["gone"] = time.Now().Add(-2 * seenModTTL)
	rs.seenMods["here"] = time.Now()
	_ = rs.voteDelayLocked("report-1")
	_, stale := rs.seenMods["gone"]
	_, live := rs.seenMods["here"]
	rs.mx.Unlock()

	require.False(t, stale)
	require.True(t, live)
}

// failingMember refuses to produce a ballot, or fails to broadcast one.
type failingMember struct {
	*stubMember
	ballotErr    error
	broadcastErr error
}

func (m *failingMember) Ballot(reportID string, subject event.ReportEvent) (vote.Event, bool, error) {
	if m.ballotErr != nil {
		return vote.Event{}, false, m.ballotErr
	}
	return m.stubMember.Ballot(reportID, subject)
}

func (m *failingMember) Broadcast(v vote.Event) error {
	if m.broadcastErr != nil {
		_ = m.stubMember.Broadcast(v)
		return m.broadcastErr
	}
	return m.stubMember.Broadcast(v)
}

func TestCastVoteToleratesMemberFailures(t *testing.T) {
	t.Run("a failed ballot casts nothing", func(t *testing.T) {
		member := &failingMember{
			stubMember: newStubMember("self", domain.OK),
			ballotErr:  errors.New("engine down"),
		}
		rs := NewRegistry("self", member, Schedule{Window: time.Hour, Failover: time.Hour, Step: 0})
		rs.Open(subject("tweet-1"))

		require.Eventually(t, func() bool {
			_, broadcasts, _ := member.counts()
			return broadcasts == 0
		}, time.Second, 10*time.Millisecond)
		rs.StopAll()
	})

	t.Run("a failed broadcast still counts the own ballot", func(t *testing.T) {
		member := &failingMember{
			stubMember:   newStubMember("self", domain.OK),
			broadcastErr: errors.New("gossip down"),
		}
		rs := NewRegistry("self", member, Schedule{Window: time.Hour, Failover: time.Hour, Step: 0})
		rs.Open(subject("tweet-1"))

		require.Eventually(t, func() bool {
			_, broadcasts, _ := member.counts()
			return broadcasts > 0
		}, 2*time.Second, 10*time.Millisecond)
		rs.StopAll()
	})
}

func TestClosedRoundIgnoresLateInput(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())
	rep := subject("tweet-1")
	id := rep.ReportID()

	// open the round without a report so nothing schedules this node's own
	// ballot: the test is about what a *closed* round accepts
	rs.mx.Lock()
	r := rs.ensureLocked(id)
	rs.mx.Unlock()
	require.NotNil(t, r)

	r.stop()

	// neither a repeat report nor a late vote may revive a stopped round
	r.setReport(rep, 0)
	r.addVote(vote.Event{ReportID: id, ModeratorID: "late"})

	r.mx.Lock()
	defer r.mx.Unlock()
	require.Empty(t, r.votes)
}

func TestRepeatReportIsIgnored(t *testing.T) {
	rs := NewRegistry("self", newStubMember("self", domain.OK), frozen())
	rep := subject("tweet-1")
	rs.Open(rep)
	// the same report delivered twice must not schedule a second vote
	rs.Open(rep)
	require.Equal(t, 1, rs.Len())
	rs.StopAll()
}

func TestDuplicateVoteIsIgnored(t *testing.T) {
	// an abstaining member never adds a ballot of its own, so the only votes
	// in the round are the ones this test delivers
	member := newStubMember("self", domain.OK)
	member.abstain = true

	rs := NewRegistry("self", member, frozen())
	rep := subject("tweet-1")
	rs.Open(rep)
	id := rep.ReportID()

	rs.AddVote(vote.Event{ReportID: id, ModeratorID: "moderator-a", Result: domain.OK})
	rs.AddVote(vote.Event{ReportID: id, ModeratorID: "moderator-a", Result: domain.FAIL})

	rs.mx.Lock()
	r := rs.active[id]
	rs.mx.Unlock()
	require.NotNil(t, r)

	r.mx.Lock()
	count := len(r.votes)
	result := r.votes["moderator-a"].Result
	r.mx.Unlock()

	// release the round lock before StopAll: stopping takes it too
	rs.StopAll()

	require.Equal(t, 1, count)
	require.Equal(t, domain.OK, result)
}
