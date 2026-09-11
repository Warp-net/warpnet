// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ratedNode = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

type stubRating struct {
	own    domain.NodeRating
	view   domain.NodeRating
	asked  warpnet.WarpPeerID
	ownErr error
	err    error
}

func (s *stubRating) Own() (domain.NodeRating, error) {
	return s.own, s.ownErr
}

func (s *stubRating) View(peerID warpnet.WarpPeerID) (domain.NodeRating, error) {
	s.asked = peerID
	return s.view, s.err
}

func ratingOf(t *testing.T, resp any) domain.NodeRating {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var got domain.NodeRating
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}

func TestOwnRatingIsWhatTheNetworkSays(t *testing.T) {
	reader := &stubRating{own: domain.NodeRating{
		NodeID: "self", Overall: 750, Tier: "watched", Observers: 2,
		Dimensions: []domain.DimensionRating{{
			Name: "net", Score: 750, Tier: "watched",
			Recent: []domain.OffenceTally{{Kind: "bad_signature", Count: 1}},
		}},
	}}

	resp, err := StreamGetOwnRatingHandler(reader)(nil, nil)
	require.NoError(t, err)

	got := ratingOf(t, resp)
	assert.EqualValues(t, 750, got.Overall)
	assert.Equal(t, "watched", got.Tier)
	assert.Equal(t, 2, got.Observers)
	require.Len(t, got.Dimensions, 1)
	assert.Equal(t, "bad_signature", got.Dimensions[0].Recent[0].Kind)
}

func TestRatingOfAPeerIsThePublicView(t *testing.T) {
	reader := &stubRating{view: domain.NodeRating{NodeID: ratedNode, Overall: 500, Tier: "watched"}}

	body, err := json.Marshal(event.GetRatingEvent{NodeId: ratedNode})
	require.NoError(t, err)

	resp, err := StreamGetRatingHandler(reader)(body, nil)
	require.NoError(t, err)

	assert.Equal(t, warpnet.FromStringToPeerID(ratedNode), reader.asked)
	assert.EqualValues(t, 500, ratingOf(t, resp).Overall)
}

func TestAskingForNoNodeAnswersWithOurOwn(t *testing.T) {
	reader := &stubRating{own: domain.NodeRating{NodeID: "self", Overall: 1000, Tier: "trusted"}}

	body, err := json.Marshal(event.GetRatingEvent{})
	require.NoError(t, err)

	resp, err := StreamGetRatingHandler(reader)(body, nil)
	require.NoError(t, err)

	assert.Equal(t, "self", ratingOf(t, resp).NodeID)
	assert.Empty(t, reader.asked, "the public view of nobody was never asked for")
}

func TestAMalformedNodeIdIsRefused(t *testing.T) {
	body, err := json.Marshal(event.GetRatingEvent{NodeId: "definitely-not-a-peer-id"})
	require.NoError(t, err)

	_, err = StreamGetRatingHandler(&stubRating{})(body, nil)
	assert.ErrorIs(t, err, warpnet.ErrMalformedNodeId)

	_, err = StreamGetRatingHandler(&stubRating{})([]byte("{"), nil)
	assert.Error(t, err, "a request that is not a request is refused")
}

func TestANodeWithNoRatingSaysSo(t *testing.T) {
	_, err := StreamGetOwnRatingHandler(nil)(nil, nil)
	assert.ErrorIs(t, err, ErrRatingUnavailable)

	_, err = StreamGetRatingHandler(nil)(nil, nil)
	assert.ErrorIs(t, err, ErrRatingUnavailable)
}

func TestAReadThatFailsIsReported(t *testing.T) {
	failing := errors.New("the store is down")

	_, err := StreamGetOwnRatingHandler(&stubRating{ownErr: failing})(nil, nil)
	assert.ErrorIs(t, err, failing)

	body, marshalErr := json.Marshal(event.GetRatingEvent{NodeId: ratedNode})
	require.NoError(t, marshalErr)
	_, err = StreamGetRatingHandler(&stubRating{err: failing})(body, nil)
	assert.ErrorIs(t, err, failing)
}
