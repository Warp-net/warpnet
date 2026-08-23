// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"crypto/ed25519"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRater struct {
	mu       sync.Mutex
	subjects []warpnet.WarpPeerID
	kinds    []rating.Kind
}

func (r *recordingRater) Record(subject warpnet.WarpPeerID, k rating.Kind) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subjects = append(r.subjects, subject)
	r.kinds = append(r.kinds, k)
	return nil
}

func (r *recordingRater) charged() []rating.Kind {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]rating.Kind(nil), r.kinds...)
}

// moderatorIdentity mints a real keypair: a verdict is only acted on
// if its signature verifies against the pubkey derived from the
// moderator's own peer id.
func moderatorIdentity(t *testing.T) (warpnet.WarpPeerID, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id, priv
}

func offenderNode(t *testing.T) warpnet.WarpPeerID {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	return id
}

func signedVerdict(
	t *testing.T, moderator warpnet.WarpPeerID, priv ed25519.PrivateKey,
	userID string, verdict domain.ModerationResult,
) []byte {
	t.Helper()
	objectID := "tweet-1"
	ev := event.ModerationVerdictEvent{
		ModeratorID: moderator.String(),
		Type:        domain.ModerationTweetType,
		UserID:      userID,
		ObjectID:    &objectID,
		Verdict:     verdict,
	}
	ev = ev.Signed(priv)
	raw, err := json.Marshal(ev)
	require.NoError(t, err)
	return raw
}

func TestUpheldVerdictChargesTheOffendersNode(t *testing.T) {
	moderator, priv := moderatorIdentity(t)
	offender := offenderNode(t)
	const userID = "offender-user"

	rater := &recordingRater{}
	users := stubModerationUserUpdater{
		getFn: func(string) (domain.User, error) {
			return domain.User{Id: userID, NodeId: offender.String()}, nil
		},
	}

	h := StreamModerationResultHandler(
		stubModerationNotifier{}, stubModerationTweetUpdater{}, users,
		stubModerationTimelineDeleter{}, stubAuth{owner: domain.Owner{UserId: "owner"}}, rater,
	)

	_, err := h(signedVerdict(t, moderator, priv, userID, domain.FAIL), s{})
	require.NoError(t, err)

	assert.Equal(t, []rating.Kind{rating.KindModerationUpheld}, rater.charged())
	assert.Equal(t, offender, rater.subjects[0],
		"the offender's node is charged, not the moderator's")
}

func TestClearedVerdictChargesNobody(t *testing.T) {
	moderator, priv := moderatorIdentity(t)
	offender := offenderNode(t)
	const userID = "cleared-user"

	rater := &recordingRater{}
	users := stubModerationUserUpdater{
		getFn: func(string) (domain.User, error) {
			return domain.User{Id: userID, NodeId: offender.String()}, nil
		},
	}

	h := StreamModerationResultHandler(
		stubModerationNotifier{}, stubModerationTweetUpdater{}, users,
		stubModerationTimelineDeleter{}, stubAuth{owner: domain.Owner{UserId: "owner"}}, rater,
	)

	_, err := h(signedVerdict(t, moderator, priv, userID, domain.OK), s{})
	require.NoError(t, err)

	assert.Empty(t, rater.charged(), "a cleared report must cost the reported node nothing")
}

func TestVerdictAboutAnUnknownUserChargesNobody(t *testing.T) {
	moderator, priv := moderatorIdentity(t)

	rater := &recordingRater{}
	users := stubModerationUserUpdater{
		getFn: func(string) (domain.User, error) {
			// An observer that never cached this user has nobody to charge.
			return domain.User{}, assert.AnError
		},
	}

	h := StreamModerationResultHandler(
		stubModerationNotifier{}, stubModerationTweetUpdater{}, users,
		stubModerationTimelineDeleter{}, stubAuth{owner: domain.Owner{UserId: "owner"}}, rater,
	)

	_, err := h(signedVerdict(t, moderator, priv, "ghost-user", domain.FAIL), s{})
	require.NoError(t, err)

	assert.Empty(t, rater.charged())
}

func TestForgedVerdictChargesNobody(t *testing.T) {
	moderator, priv := moderatorIdentity(t)
	impostor := offenderNode(t)

	rater := &recordingRater{}
	users := stubModerationUserUpdater{
		getFn: func(string) (domain.User, error) {
			return domain.User{Id: "u", NodeId: impostor.String()}, nil
		},
	}

	raw := signedVerdict(t, moderator, priv, "u", domain.FAIL)
	var ev event.ModerationVerdictEvent
	require.NoError(t, json.Unmarshal(raw, &ev))
	ev.ModeratorID = impostor.String() // same verdict, forged attribution
	forged, err := json.Marshal(ev)
	require.NoError(t, err)

	h := StreamModerationResultHandler(
		stubModerationNotifier{}, stubModerationTweetUpdater{}, users,
		stubModerationTimelineDeleter{}, stubAuth{owner: domain.Owner{UserId: "owner"}}, rater,
	)

	_, err = h(forged, s{})
	require.Error(t, err)
	assert.Empty(t, rater.charged(),
		"a verdict that fails signature verification must not move anyone's rating")
}
