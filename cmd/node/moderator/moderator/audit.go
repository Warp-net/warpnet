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

package moderator

import (
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/audit"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	log "github.com/sirupsen/logrus"
)

const (
	// auditInterval is how often this node spot-checks a random peer. One
	// challenge costs the peer a single inference, so this can stay
	// frequent enough to build a picture within a day.
	auditInterval = 5 * time.Minute
	// judgedCapacity bounds the texts held between casting a ballot and
	// the round deciding. Rounds close within a minute; this is slack for
	// a burst of reports, not a cache.
	judgedCapacity = 512
)

// ChallengeHandler answers audit spot-checks from other moderators: it runs
// the local engine over the challenged text and signs the answer with this
// node's key. Register it under audit.ChallengeRoute.
func (m *Moderator) ChallengeHandler() warpnet.WarpHandlerFunc {
	return audit.StreamChallengeHandler(
		engine,
		audit.NewResponseSigner(m.privKey, m.selfID(), supportedModel.Model),
	)
}

// AuditStanding reports what this node's own spot-checks make of a peer.
// Nothing consults it yet — see the audit package docs on why a single
// auditor must not be allowed to disqualify anyone on its own.
func (m *Moderator) AuditStanding(peerID string) audit.Standing {
	return m.ledger.StandingOf(peerID)
}

// runAudits spot-checks a random peer on a timer for as long as the node
// runs. It is best-effort background work: an audit that finds nobody to
// ask, or no reference to ask about, simply does nothing this tick.
func (m *Moderator) runAudits() {
	ticker := time.NewTicker(auditInterval)
	defer ticker.Stop()

	var done <-chan struct{}
	if m.ctx != nil {
		done = m.ctx.Done()
	}

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if m.isClosed.Load() {
				return
			}
			res := m.auditor.ChallengeRandomPeer(m.rounds.Peers())
			if res == nil {
				continue
			}
			if standing := m.ledger.StandingOf(res.Peer); standing != audit.StandingTrusted {
				log.Infof("moderator: audit %s outcome=%d standing=%s", res.Peer, res.Outcome, standing)
			}
		}
	}
}

// holdJudged remembers the text this node fed to the engine for a round,
// until that round decides and the text can be filed as a reference.
func (m *Moderator) holdJudged(reportID, text string) {
	if text == "" {
		return
	}
	m.judgedMx.Lock()
	defer m.judgedMx.Unlock()
	// A flood of reports must not grow this without bound; dropping the
	// oldest slot costs at most one missed reference.
	if len(m.judged) >= judgedCapacity {
		for id := range m.judged {
			delete(m.judged, id)
			break
		}
	}
	m.judged[reportID] = text
}

// fileReference turns a decided round into audit material: the text this
// node judged, labelled with the verdict the quorum reached. The quorum's
// answer is the reference precisely because it is not this node's own
// opinion. Called on every decided round, whether this node carried the
// decision or merely heard the announcement.
func (m *Moderator) fileReference(reportID string, verdict domain.ModerationResult) {
	m.judgedMx.Lock()
	text, ok := m.judged[reportID]
	delete(m.judged, reportID)
	m.judgedMx.Unlock()

	if !ok {
		// This node never voted on that round, so it never fetched the
		// text and has nothing to file.
		return
	}
	m.corpus.Remember(text, verdict)
}
