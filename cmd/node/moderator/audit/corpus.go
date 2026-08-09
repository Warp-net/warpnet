/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package audit

import (
	"math/rand"
	"slices"
	"sync"

	"github.com/Warp-net/warpnet/domain"
)

// corpusPerClass is how many references of each class a node keeps. Two
// classes, so the corpus holds at most twice this many texts.
const corpusPerClass = 128

// Corpus is the reference material an audit draws on: real texts a vote
// round has already ruled on, with the quorum's verdict as ground truth.
//
// It replaces the synthetic probe list an earlier draft carried. A fixed
// list of made-up texts is worthless as a challenge: the code is public,
// so anyone can tabulate "this template means unsafe" and answer correctly
// with no model at all. References taken from live traffic cannot be
// tabulated ahead of time — the auditor picks from an ever-changing pool
// only it has seen — and they cost nothing to produce, since the round
// already fetched the text and already agreed on the verdict.
//
// Keeping the two classes in separate rings matters: a challenger draws
// them in equal measure, so a peer that answers a constant class scores
// about half and lands under the ban threshold either way.
type Corpus struct {
	mu     sync.Mutex
	safe   []string
	unsafe []string
	// next is the round-robin write cursor per class.
	nextSafe   int
	nextUnsafe int
}

func NewCorpus() *Corpus {
	return &Corpus{
		safe:   make([]string, 0, corpusPerClass),
		unsafe: make([]string, 0, corpusPerClass),
	}
}

// Remember files a text a round has ruled on. Call it when a round
// decides, with the text that was judged and the quorum's verdict: the
// agreement of several independent moderators is a stronger reference than
// any single node's opinion, which is exactly what an audit needs.
//
// Texts already in the corpus are not re-filed, so a report storm over one
// tweet cannot flood the references with copies of it.
func (c *Corpus) Remember(text string, verdict domain.ModerationResult) {
	if text == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	ring, cursor := &c.safe, &c.nextSafe
	if !bool(verdict) {
		ring, cursor = &c.unsafe, &c.nextUnsafe
	}
	if slices.Contains(*ring, text) {
		return
	}
	if len(*ring) < corpusPerClass {
		*ring = append(*ring, text)
		return
	}
	(*ring)[*cursor] = text
	*cursor = (*cursor + 1) % corpusPerClass
}

// Sample draws one reference, alternating classes at random so neither is
// predictable. The bool is false while either class is still empty: until
// this node has seen both a clean and a moderated text there is nothing to
// audit anyone against, and guessing would be worse than waiting.
func (c *Corpus) Sample(rng *rand.Rand) (text string, expectUnsafe bool, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.safe) == 0 || len(c.unsafe) == 0 {
		return "", false, false
	}
	if rng.Intn(2) == 0 {
		return c.safe[rng.Intn(len(c.safe))], false, true
	}
	return c.unsafe[rng.Intn(len(c.unsafe))], true, true
}

// Len reports how many references are on hand per class.
func (c *Corpus) Len() (safe, unsafe int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.safe), len(c.unsafe)
}
