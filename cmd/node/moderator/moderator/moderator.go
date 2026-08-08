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
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
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

// VoteExchange carries the per-round moderator votes over gossip.
type VoteExchange interface {
	PublishVote(ev event.ModerationVoteEvent) error
	SubscribeVotes(h func(ev event.ModerationVoteEvent) error) error
}

// Moderator runs entirely report-driven: there is no peer-scanning loop.
// Every report opens a vote round shared by all moderators listening on
// ReportsTopic; the round machinery in votes.go decides who assesses the
// content, tallies the votes and lets the round's chair publish the
// aggregate verdict.
type Moderator struct {
	ctx       context.Context
	node      ModeratorNode
	sub       ReportSubscriber
	votes     VoteExchange
	isolation *isolation.IsolationProtocol
	privKey   ed25519.PrivateKey

	mx        sync.Mutex
	rounds    map[string]*voteRound
	finalized map[string]time.Time
	seenMods  map[string]time.Time

	isClosed *atomic.Bool
}

func NewModerator(
	ctx context.Context,
	node ModeratorNode,
	pub Publisher,
	sub ReportSubscriber,
	votes VoteExchange,
	privKey ed25519.PrivateKey,
) (*Moderator, error) {
	m := &Moderator{
		ctx:       ctx,
		node:      node,
		sub:       sub,
		votes:     votes,
		privKey:   privKey,
		rounds:    make(map[string]*voteRound),
		finalized: make(map[string]time.Time),
		seenMods:  make(map[string]time.Time),
		isClosed:  new(atomic.Bool),
	}
	m.isolation = isolation.NewIsolationProtocol(pub, m.signResult)
	return m, nil
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
	if err := m.votes.SubscribeVotes(m.handleVote); err != nil {
		return fmt.Errorf("moderator: subscribe votes: %w", err)
	}

	log.Infoln("moderator: started (report-driven)")
	return nil
}

func (m *Moderator) Close() {
	m.isClosed.Store(true)

	m.mx.Lock()
	for id, r := range m.rounds {
		r.stopTimersLocked()
		delete(m.rounds, id)
	}
	m.mx.Unlock()

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
		if ev.ObjectID == nil || *ev.ObjectID == "" {
			log.Warn("moderator: tweet report missing object_id")
			return nil
		}
	case domain.ModerationUserType:
	default:
		// ValidateReport already rejects unsupported types; this
		// branch is defensive in case the allowlist grows later.
		return nil
	}

	m.openRound(ev)
	return nil
}

// signResult stamps and signs a verdict with this node's key so member
// nodes can verify it really came from the moderator named in ModeratorID.
func (m *Moderator) signResult(ev *event.ModerationResultEvent) {
	ev.TimeAt = time.Now().UTC()
	if len(m.privKey) == 0 {
		return
	}
	ev.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(m.privKey, ev.SigningBytes()))
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
	voters []domain.ID,
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
		Voters:      voters,
	}
	m.signResult(&result)
	if _, err := m.node.GenericStream(
		rep.ReporterNodeID,
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	); err != nil {
		log.Warnf("moderator: notify reporter %s: %v", rep.ReporterNodeID, err)
	}
}

const fetchAttempts = 3

var (
	fetchRetryDelay = 3 * time.Second

	ErrFetchFailed = errors.New("moderator: fetch failed")
)

func (m *Moderator) fetchObject(nodeID string, route stream.WarpRoute, payload any) ([]byte, error) {
	var done <-chan struct{}
	if m.ctx != nil {
		done = m.ctx.Done()
	}

	var lastErr error
	for attempt := range fetchAttempts {
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
			lastErr = fmt.Errorf("%w: %s", ErrFetchFailed, respErr.Message)
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

// verdict is one moderator's assessment of a report, before the round tally.
type verdict struct {
	result   domain.ModerationResult
	reason   *string
	objectID *domain.ID
	userID   domain.ID
}

// unavailableVerdict is the vote for content that could not be reviewed:
// an OK (no-op) result with a sentinel reason the reporter recognises.
func unavailableVerdict(objectID *domain.ID, userID domain.ID) verdict {
	reason := event.ModerationReasonUnavailable
	return verdict{result: domain.OK, reason: &reason, objectID: objectID, userID: userID}
}

// assessReport produces this moderator's own vote on a report. The bool is
// false when the report is malformed and no vote should be cast at all.
func (m *Moderator) assessReport(ev event.ReportEvent) (verdict, bool, error) {
	switch ev.Type {
	case domain.ModerationTweetType:
		return m.assessTweetReport(ev)
	case domain.ModerationUserType:
		return m.assessUserReport(ev)
	default:
		return verdict{}, false, nil
	}
}

func (m *Moderator) assessTweetReport(ev event.ReportEvent) (verdict, bool, error) {
	if ev.ObjectID == nil || *ev.ObjectID == "" {
		log.Warn("moderator: tweet report missing object_id")
		return verdict{}, false, nil
	}

	data, err := m.fetchObject(
		ev.TargetNodeID,
		event.PUBLIC_GET_TWEET,
		event.GetTweetEvent{TweetId: *ev.ObjectID, UserId: ev.TargetUserID},
	)
	if err != nil {
		log.Warnf("moderator: fetch tweet %s from node %s failed: %v", *ev.ObjectID, ev.TargetNodeID, err)
		return unavailableVerdict(ev.ObjectID, ev.TargetUserID), true, nil
	}

	var tweet domain.Tweet
	if err := json.Unmarshal(data, &tweet); err != nil {
		return verdict{}, false, fmt.Errorf("moderator: unmarshal tweet: %w", err)
	}
	if tweet.Id == "" {
		log.Warnf("moderator: tweet %s not found on node %s", *ev.ObjectID, ev.TargetNodeID)
		return unavailableVerdict(ev.ObjectID, ev.TargetUserID), true, nil
	}
	if tweet.Text == "" {
		log.Infof("moderator: tweet %s has no text to moderate", tweet.Id)
		return unavailableVerdict(&tweet.Id, tweet.UserId), true, nil
	}

	if tweet.Moderation != nil {
		log.Infof("moderator: tweet %s already moderated ok=%t, reusing verdict",
			tweet.Id, bool(tweet.Moderation.IsOk))
		return verdict{
			result:   tweet.Moderation.IsOk,
			reason:   tweet.Moderation.Reason,
			objectID: &tweet.Id,
			userID:   tweet.UserId,
		}, true, nil
	}

	ok, reason, err := engine.Moderate(tweet.Text)
	if err != nil {
		return verdict{}, false, fmt.Errorf("moderator: process tweet: %w", err)
	}
	log.Infof("moderator: tweet verdict tweet=%s ok=%t", tweet.Id, ok)
	return verdict{
		result:   domain.ModerationResult(ok),
		reason:   &reason,
		objectID: &tweet.Id,
		userID:   tweet.UserId,
	}, true, nil
}

func (m *Moderator) assessUserReport(ev event.ReportEvent) (verdict, bool, error) {
	data, err := m.fetchObject(
		ev.TargetNodeID,
		event.PUBLIC_GET_USER,
		event.GetUserEvent{UserId: ev.TargetUserID},
	)
	if err != nil {
		log.Warnf("moderator: fetch user %s from node %s failed: %v", ev.TargetUserID, ev.TargetNodeID, err)
		return unavailableVerdict(nil, ev.TargetUserID), true, nil
	}

	var user domain.User
	if err := json.Unmarshal(data, &user); err != nil {
		return verdict{}, false, fmt.Errorf("moderator: unmarshal user: %w", err)
	}
	if user.Id == "" {
		log.Warnf("moderator: user %s not found on node %s", ev.TargetUserID, ev.TargetNodeID)
		return unavailableVerdict(nil, ev.TargetUserID), true, nil
	}

	text := buildProfileText(user)
	if text == "" {
		log.Warn("moderator: empty profile text")
		return unavailableVerdict(nil, user.Id), true, nil
	}

	ok, reason, err := engine.Moderate(text)
	if err != nil {
		return verdict{}, false, fmt.Errorf("moderator: process user: %w", err)
	}
	log.Infof("moderator: user verdict user=%s ok=%t", user.Id, ok)
	return verdict{result: domain.ModerationResult(ok), reason: &reason, userID: user.Id}, true, nil
}

// finalizeRound is executed by the round's chair only: it delivers the
// aggregate verdict to the reporter and, on FAIL, runs the isolation
// broadcast. Shadow-ban: only bad verdicts go on the followers broadcast.
func (m *Moderator) finalizeRound(rep event.ReportEvent, agg verdict, voters []domain.ID) {
	m.notifyReporter(rep, agg.result, agg.reason, agg.objectID, agg.userID, voters)

	if bool(agg.result) {
		return
	}

	switch rep.Type {
	case domain.ModerationTweetType:
		if agg.objectID == nil {
			return
		}
		m.isolation.IsolateTweet(
			&domain.Tweet{Id: *agg.objectID, UserId: agg.userID},
			&domain.TweetModeration{
				ModeratorID: m.node.ID().String(),
				Model:       domain.LLAMAGuard3,
				IsOk:        domain.FAIL,
				Reason:      agg.reason,
				TimeAt:      time.Now(),
			},
			voters,
		)
	case domain.ModerationUserType:
		m.isolation.IsolateUser(
			m.node.ID().String(),
			&domain.User{Id: agg.userID},
			&domain.UserModeration{
				IsModerated: true,
				Model:       domain.LLAMAGuard3,
				IsOk:        false,
				Reason:      agg.reason,
				TimeAt:      time.Now(),
			},
			voters,
		)
	}
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
