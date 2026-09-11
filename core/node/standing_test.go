// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// taggingPrioritizer records what a node decided a peer is worth.
type taggingPrioritizer struct {
	mu   sync.Mutex
	tags map[string]int
}

func newTaggingPrioritizer() *taggingPrioritizer {
	return &taggingPrioritizer{tags: make(map[string]int)}
}

func (p *taggingPrioritizer) SetRatingPriority(pid warpnet.WarpPeerID, tag int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tags[pid.String()] = tag
}

func (p *taggingPrioritizer) tagged(pid warpnet.WarpPeerID) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tag, ok := p.tags[pid.String()]
	return tag, ok
}

func (p *taggingPrioritizer) SetPriority(warpnet.WarpPeerID, warpnet.WarpReachability) {}
func (p *taggingPrioritizer) SetMinPriority(warpnet.WarpPeerID)                        {}
func (p *taggingPrioritizer) SetMaxPriority(warpnet.WarpPeerID)                        {}

func TestAStandingIsWhatAPeerIsWorthToTheConnectionManager(t *testing.T) {
	prioritizer := newTaggingPrioritizer()
	n := &WarpNode{prioritizer: prioritizer}
	peerID := warpnet.FromStringToPeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo")

	n.Apply(warpnet.PeerStanding{PeerID: peerID.String(), ConnTag: 10})

	tag, ok := prioritizer.tagged(peerID)
	require.True(t, ok)
	assert.Equal(t, 10, tag)

	n.Apply(warpnet.PeerStanding{PeerID: peerID.String(), ConnTag: 60})
	tag, _ = prioritizer.tagged(peerID)
	assert.Equal(t, 60, tag, "a standing that recovers is worth more again")
}

func TestApplyIgnoresAStandingThatNamesNoPeer(t *testing.T) {
	prioritizer := newTaggingPrioritizer()
	n := &WarpNode{prioritizer: prioritizer}

	n.Apply(warpnet.PeerStanding{ConnTag: 1})
	n.Apply(warpnet.PeerStanding{PeerID: "not-a-peer-id", ConnTag: 1})

	assert.Empty(t, prioritizer.tags)

	var nilNode *WarpNode
	assert.NotPanics(t, func() { nilNode.Apply(warpnet.PeerStanding{PeerID: "peer", ConnTag: 1}) })
	assert.NotPanics(t, func() { (&WarpNode{}).Apply(warpnet.PeerStanding{PeerID: "peer"}) })
}
