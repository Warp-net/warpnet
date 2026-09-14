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

const taggedPeer = "12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"

// worth answers what it was told a peer is worth to the connection manager.
type worth map[string]int

func (w worth) ConnTag(peerID warpnet.WarpPeerID) int { return w[peerID.String()] }

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

func TestAPeerIsWorthToTheConnectionManagerWhatItIsRated(t *testing.T) {
	prioritizer := newTaggingPrioritizer()
	ratings := worth{taggedPeer: 10}
	n := &WarpNode{prioritizer: prioritizer, ratings: ratings}
	peerID := warpnet.FromStringToPeerID(taggedPeer)

	n.tagPeer(peerID)

	tag, ok := prioritizer.tagged(peerID)
	require.True(t, ok)
	assert.Equal(t, 10, tag)

	ratings[taggedPeer] = 60
	n.tagPeer(peerID)
	tag, _ = prioritizer.tagged(peerID)
	assert.Equal(t, 60, tag, "a rating that recovers is worth more again")
}

func TestANodeWithNoRatingsTagsNobody(t *testing.T) {
	prioritizer := newTaggingPrioritizer()
	peerID := warpnet.FromStringToPeerID(taggedPeer)

	n := &WarpNode{prioritizer: prioritizer}
	n.tagPeer(peerID)
	assert.Empty(t, prioritizer.tags, "a node with no ratings leaves the tag alone")

	rated := &WarpNode{prioritizer: prioritizer, ratings: worth{}}
	rated.tagPeer("")
	assert.Empty(t, prioritizer.tags, "and a peer with no id is nobody to tag")

	var nilNode *WarpNode
	assert.NotPanics(t, func() { nilNode.tagPeer(peerID) })
}
