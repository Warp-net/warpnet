//nolint:all
package event

import (
	"bytes"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/domain"
)

func baseResultEvent() ModerationVerdictEvent {
	reason := "Hate"
	objectID := domain.ID("tweet-1")
	return ModerationVerdictEvent{
		Type:        domain.ModerationTweetType,
		Verdict:     domain.FAIL,
		Reason:      &reason,
		Model:       domain.LLAMAGuard3,
		UserID:      "offender",
		ObjectID:    &objectID,
		ModeratorID: "moderator-1",
		ReporterID:  "reporter-1",
		Voters:      []domain.ID{"moderator-1", "moderator-2", "moderator-3"},
		TimeAt:      time.Unix(1700000000, 42).UTC(),
	}
}

// Every signed field must change the signing bytes: a verdict altered after
// signing (result flipped, reporter redirected, voters trimmed) must not
// verify against the original signature.
func TestModerationResultSigningBytes_CoversEveryField(t *testing.T) {
	baseEvent := baseResultEvent()
	base := baseEvent.signingBytes()

	mutations := map[string]func(*ModerationVerdictEvent){
		"type":     func(e *ModerationVerdictEvent) { e.Type = domain.ModerationUserType },
		"result":   func(e *ModerationVerdictEvent) { e.Verdict = domain.OK },
		"reason":   func(e *ModerationVerdictEvent) { r := "Spam"; e.Reason = &r },
		"model":    func(e *ModerationVerdictEvent) { e.Model = domain.ModelType("other") },
		"user":     func(e *ModerationVerdictEvent) { e.UserID = "someone-else" },
		"object":   func(e *ModerationVerdictEvent) { o := domain.ID("tweet-2"); e.ObjectID = &o },
		"moder":    func(e *ModerationVerdictEvent) { e.ModeratorID = "moderator-x" },
		"reporter": func(e *ModerationVerdictEvent) { e.ReporterID = "reporter-x" },
		"voters":   func(e *ModerationVerdictEvent) { e.Voters = e.Voters[:2] },
		"time":     func(e *ModerationVerdictEvent) { e.TimeAt = e.TimeAt.Add(time.Nanosecond) },
	}
	for name, mutate := range mutations {
		ev := baseResultEvent()
		mutate(&ev)
		if bytes.Equal(base, ev.signingBytes()) {
			t.Fatalf("mutating %s must change the signing bytes", name)
		}
	}
}

// The signature field itself must not feed the signing bytes, and nil
// pointers must be handled.
func TestModerationResultSigningBytes_StableAndNilSafe(t *testing.T) {
	ev := baseResultEvent()
	before := ev.signingBytes()
	ev.Signature = "c2lnbmF0dXJl"
	if !bytes.Equal(before, ev.signingBytes()) {
		t.Fatal("the signature field must not change the signing bytes")
	}

	minimal := ModerationVerdictEvent{Type: domain.ModerationUserType, UserID: "u"}
	if len(minimal.signingBytes()) == 0 {
		t.Fatal("nil reason/object/voters must still produce signing bytes")
	}
}

func TestReportID_StableAndDistinct(t *testing.T) {
	objectID := domain.ID("tweet-1")
	rep := ReportEvent{
		Type:           domain.ModerationTweetType,
		TargetUserID:   "offender",
		TargetNodeID:   "node-1",
		ObjectID:       &objectID,
		Reason:         "Hate",
		ReporterID:     "reporter-1",
		ReporterNodeID: "reporter-node-1",
	}

	if rep.ReportID() != rep.ReportID() {
		t.Fatal("the report id must be deterministic")
	}

	other := rep
	otherObject := domain.ID("tweet-2")
	other.ObjectID = &otherObject
	if rep.ReportID() == other.ReportID() {
		t.Fatal("different objects must produce different rounds")
	}

	reReport := rep
	reReport.ReporterID = "reporter-2"
	if rep.ReportID() == reReport.ReportID() {
		t.Fatal("a different reporter opens its own round")
	}

	// Field-boundary aliasing: shifting a byte across two adjacent fields
	// must not collide thanks to the length prefixes.
	a := rep
	a.TargetUserID = "offenderX"
	a.TargetNodeID = "node-1"
	b := rep
	b.TargetUserID = "offender"
	b.TargetNodeID = "Xnode-1"
	if a.ReportID() == b.ReportID() {
		t.Fatal("length prefixes must prevent adjacent-field aliasing")
	}
}
