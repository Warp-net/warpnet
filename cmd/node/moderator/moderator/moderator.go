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
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/moderator/isolation"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	ErrModeratorInitFailed warpnet.WarpError = "failed to init moderator engine"
)

type Engine interface {
	Moderate(content string) (bool, string, error)
	Close()
}

// build constrained
var (
	engine          Engine
	engineReadyChan = make(chan struct{}, 1)
)

type ModeratorNode interface {
	Node() warpnet.P2PNode
	ID() warpnet.WarpPeerID
	NodeInfo() warpnet.NodeInfo
	GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error)
}

type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, body any) (err error)
}

// ReportSubscriber is the slice of the moderator pubsub the Moderator
// needs. It hands out one ReportEvent per gossip message.
type ReportSubscriber interface {
	SubscribeReports(h func(ev event.ReportEvent) error) error
}

// Moderator now runs entirely report-driven: there is no peer-scanning
// loop. Every Moderate() call originates from a Report published on
// ReportsTopic by some member node.
type Moderator struct {
	ctx       context.Context
	node      ModeratorNode
	sub       ReportSubscriber
	isolation *isolation.IsolationProtocol

	isClosed *atomic.Bool
}

func NewModerator(
	ctx context.Context,
	node ModeratorNode,
	pub Publisher,
	sub ReportSubscriber,
) (*Moderator, error) {
	return &Moderator{
		ctx:       ctx,
		node:      node,
		sub:       sub,
		isolation: isolation.NewIsolationProtocol(pub),
		isClosed:  new(atomic.Bool),
	}, nil
}

func (m *Moderator) Start() error {
	if m == nil {
		panic("moderator: nil")
	}

	log.Infoln("moderator: wait engine init...")

	engineReadyChan <- struct{}{}
	<-engineReadyChan
	if engine == nil {
		return ErrModeratorInitFailed
	}
	log.Infoln("moderator: engine is running")

	if err := m.sub.SubscribeReports(m.handleReport); err != nil {
		return fmt.Errorf("moderator: subscribe reports: %w", err)
	}

	log.Infoln("moderator: started (report-driven)")
	return nil
}

func (m *Moderator) Close() {
	m.isClosed.Store(true)

	if engine != nil {
		engine.Close()
	}
}

func (m *Moderator) handleReport(ev event.ReportEvent) error {
	if m.isClosed.Load() {
		return nil
	}

	// %q quotes and escapes control characters so a reason like
	// "spam\nfake log line" can't inject log noise.
	objectID := ""
	if ev.ObjectID != nil {
		objectID = *ev.ObjectID
	}
	log.Infof("moderator: report received type=%s target_user=%s target_node=%s object_id=%s reason=%q",
		ev.Type.String(), ev.TargetUserID, ev.TargetNodeID, objectID, ev.Reason)

	switch ev.Type {
	case domain.ModerationTweetType:
		return m.handleTweetReport(ev)
	case domain.ModerationUserType:
		return m.handleUserReport(ev)
	default:
		// ValidateReport already rejects unsupported types; this
		// branch is defensive in case the allowlist grows later.
		return nil
	}
}

// notifyReporter re-sends the verdict to the reporter's node on the same
// route as the broadcast, but with ReporterID set so it notifies them.
// Unlike the followers broadcast (FAIL-only, shadow-ban), the reporter is
// told about both outcomes — silence on an OK verdict reads as "the report
// was lost". Best-effort: a delivery failure must not abort moderation.
func (m *Moderator) notifyReporter(
	rep event.ReportEvent,
	verdict domain.ModerationResult,
	reason *string,
	objectID *domain.ID,
	targetUserID domain.ID,
) {
	if rep.ReporterNodeID == "" || rep.ReporterID == "" {
		return
	}
	result := event.ModerationResultEvent{
		Type:        rep.Type,
		Result:      verdict,
		Reason:      reason,
		Model:       domain.LLAMAGuard3,
		UserID:      targetUserID,
		ObjectID:    objectID,
		ModeratorID: m.node.ID().String(),
		ReporterID:  rep.ReporterID,
	}
	if _, err := m.node.GenericStream(
		rep.ReporterNodeID,
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	); err != nil {
		log.Warnf("moderator: notify reporter %s: %v", rep.ReporterNodeID, err)
	}
}

const fetchAttempts = 3

// fetchRetryDelay is a var so tests can zero it out.
var fetchRetryDelay = 3 * time.Second

// fetchObject dials the target node with a few short retries: the origin
// node may be restarting or re-connecting right when the report arrives, and
// a report is fire-and-forget — there is no second chance once this returns.
// A failed fetch (not found, offline-forward) arrives as an
// event.ResponseError envelope, not a transport error; both count as misses.
func (m *Moderator) fetchObject(nodeID string, route stream.WarpRoute, payload any) ([]byte, error) {
	var done <-chan struct{}
	if m.ctx != nil {
		done = m.ctx.Done()
	}

	var lastErr error
	for attempt := 0; attempt < fetchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-done:
				return nil, m.ctx.Err()
			case <-time.After(fetchRetryDelay):
			}
		}

		data, err := m.node.GenericStream(nodeID, route, payload)
		if err != nil {
			lastErr = err
			continue
		}
		var respErr event.ResponseError
		if json.Unmarshal(data, &respErr) == nil && respErr.Message != "" {
			lastErr = errors.New(respErr.Message)
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// notifyUnreviewable tells the reporter their report cannot be reviewed —
// without it a lost fetch reads as a lost report. The OK result keeps every
// receiver from touching local state; the sentinel reason lets member nodes
// word the notification honestly.
func (m *Moderator) notifyUnreviewable(rep event.ReportEvent, objectID *domain.ID, targetUserID domain.ID) {
	reason := event.ModerationReasonUnavailable
	m.notifyReporter(rep, domain.OK, &reason, objectID, targetUserID)
}

func (m *Moderator) handleTweetReport(ev event.ReportEvent) error {
	if ev.ObjectID == nil || *ev.ObjectID == "" {
		log.Warn("moderator: tweet report missing object_id")
		return nil
	}

	data, err := m.fetchObject(
		ev.TargetNodeID,
		event.PUBLIC_GET_TWEET,
		event.GetTweetEvent{TweetId: *ev.ObjectID, UserId: ev.TargetUserID},
	)
	if err != nil {
		log.Warnf("moderator: fetch tweet %s from node %s failed: %v", *ev.ObjectID, ev.TargetNodeID, err)
		m.notifyUnreviewable(ev, ev.ObjectID, ev.TargetUserID)
		return nil
	}

	var tweet domain.Tweet
	if err := json.Unmarshal(data, &tweet); err != nil {
		return fmt.Errorf("moderator: unmarshal tweet: %w", err)
	}
	if tweet.Id == "" {
		log.Warnf("moderator: tweet %s not found on node %s", *ev.ObjectID, ev.TargetNodeID)
		m.notifyUnreviewable(ev, ev.ObjectID, ev.TargetUserID)
		return nil
	}
	if tweet.Text == "" {
		log.Infof("moderator: tweet %s has no text to moderate", tweet.Id)
		m.notifyUnreviewable(ev, &tweet.Id, tweet.UserId)
		return nil
	}

	// A tweet that already carries a verdict was moderated on an earlier
	// report — reuse it instead of burning another model run. The followers
	// broadcast for a FAIL went out back then; only the reporter is new here.
	if tweet.Moderation != nil {
		log.Infof("moderator: tweet %s already moderated ok=%t, reusing verdict",
			tweet.Id, bool(tweet.Moderation.IsOk))
		m.notifyReporter(ev, tweet.Moderation.IsOk, tweet.Moderation.Reason, &tweet.Id, tweet.UserId)
		return nil
	}

	ok, reason, err := engine.Moderate(tweet.Text)
	if err != nil {
		return fmt.Errorf("moderator: process tweet: %w", err)
	}
	log.Infof("moderator: tweet verdict tweet=%s ok=%t", tweet.Id, ok)

	m.notifyReporter(ev, domain.ModerationResult(ok), &reason, &tweet.Id, tweet.UserId)

	// Shadow-ban: only bad verdicts go on the followers broadcast.
	if ok {
		return nil
	}

	m.isolation.IsolateTweet(&tweet, &domain.TweetModeration{
		ModeratorID: m.node.ID().String(),
		Model:       domain.LLAMAGuard3,
		IsOk:        domain.FAIL,
		Reason:      &reason,
		TimeAt:      time.Now(),
	})
	return nil
}

func (m *Moderator) handleUserReport(ev event.ReportEvent) error {
	data, err := m.fetchObject(
		ev.TargetNodeID,
		event.PUBLIC_GET_USER,
		event.GetUserEvent{UserId: ev.TargetUserID},
	)
	if err != nil {
		log.Warnf("moderator: fetch user %s from node %s failed: %v", ev.TargetUserID, ev.TargetNodeID, err)
		m.notifyUnreviewable(ev, nil, ev.TargetUserID)
		return nil
	}

	var user domain.User
	if err := json.Unmarshal(data, &user); err != nil {
		return fmt.Errorf("moderator: unmarshal user: %w", err)
	}
	if user.Id == "" {
		log.Warnf("moderator: user %s not found on node %s", ev.TargetUserID, ev.TargetNodeID)
		m.notifyUnreviewable(ev, nil, ev.TargetUserID)
		return nil
	}

	text := buildProfileText(user)
	if text == "" {
		log.Warn("moderator: empty profile text")
		m.notifyUnreviewable(ev, nil, user.Id)
		return nil
	}

	ok, reason, err := engine.Moderate(text)
	if err != nil {
		return fmt.Errorf("moderator: process user: %w", err)
	}
	log.Infof("moderator: user verdict user=%s ok=%t", user.Id, ok)

	m.notifyReporter(ev, domain.ModerationResult(ok), &reason, nil, user.Id)

	// Shadow-ban: only bad verdicts go on the followers broadcast.
	if ok {
		return nil
	}

	m.isolation.IsolateUser(m.node.ID().String(), &user, &domain.UserModeration{
		IsModerated: true,
		Model:       domain.LLAMAGuard3,
		IsOk:        false,
		Reason:      &reason,
		TimeAt:      time.Now(),
	})
	return nil
}

func buildProfileText(u domain.User) string {
	parts := []string{u.Username, u.Bio}
	if u.Website != nil {
		parts = append(parts, *u.Website)
	}

	keys := make([]string, 0, len(u.Metadata))
	for k := range u.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, k+": "+u.Metadata[k])
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
