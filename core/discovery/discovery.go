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

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type DiscoveryHandler func(warpnet.WarpAddrInfo)

type DiscoveryInfoStorer interface {
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
	SimpleConnect(warpnet.WarpAddrInfo) error
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
	SetNodePriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability)
	SetMaxNodePriority(pid warpnet.WarpPeerID)
	SetMinNodePriority(pid warpnet.WarpPeerID)
}

// BackoffConnector dials through the node's backoff, so a dead peer
// that gossip keeps republishing is not redialled forever.
type BackoffConnector interface {
	Connect(warpnet.WarpAddrInfo) error
}

// probeInterval is how long a peer stays "recently probed". Within it
// we do not ask the same peer for its info again, however many times
// gossip, mDNS and the DHT rediscover it. Without this the network
// spent O(N²) info requests re-learning what it already knew, and any
// node answering an info request discovered its asker and asked back.
const probeInterval = 30 * time.Minute

type NodeStorer interface {
	BlocklistRemove(peerId string) error
	IsBlocklisted(peerId string) bool
	BlocklistExponential(peerId string) error
	BlocklistTerm(peerId string) (*database.BlocklistTerm, error)
}

type UserStorer interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type MetricsOnlineDiscoverer interface {
	PushStatusOnline(nodeId string)
	PushStatusOffline(nodeId string)
}

type discoverySource string

const (
	sourceStream = discoverySource("stream")
	sourceGossip = discoverySource("gossip")
	sourceMDNS   = discoverySource("mdns")
	sourceDHT    = discoverySource("dht")
)

type discoveredPeer struct {
	ID     warpnet.WarpPeerID
	Addrs  []warpnet.WarpAddress
	Source discoverySource
}

type discoveryService struct {
	ctx      context.Context
	node     DiscoveryInfoStorer
	userRepo UserStorer
	nodeRepo NodeStorer

	ownId warpnet.WarpPeerID
	// Two budgets, because one cannot do both jobs. limiter caps how
	// much discovery work this node accepts in total, whoever it comes
	// from; peerLimiter divides that budget fairly and is where a bad
	// standing costs a peer its share. A global bucket alone cannot
	// tell "twelve new peers" from "one peer twelve times", and a
	// per-peer bucket alone puts no ceiling on the sum.
	limiter     *leakyBucketRateLimiter
	peerLimiter *peerLimiter

	// channel is needed to collect discoveries while node is setting up
	discoveryChan   chan discoveredPeer
	discoveryTicker *time.Ticker
	stopChan        chan struct{}

	aliasCache *expirable.LRU[warpnet.WarpPeerID, warpnet.WarpPeerID]
	// probed remembers who we already asked for info recently, so
	// rediscovering a known peer costs nothing.
	probed *expirable.LRU[warpnet.WarpPeerID, struct{}]

	// rater is swapped rather than assigned: the mDNS, DHT and gossip
	// callbacks are already running by the time the rating store is
	// built, so they read it concurrently with SetRating.
	rater atomic.Pointer[raterHolder]

	m MetricsOnlineDiscoverer
}

type raterHolder struct{ rater rating.Rater }

// SetRating attaches the node's rating store: discovery both reports
// flooders and gives worse-rated peers a smaller share of the budget.
func (s *discoveryService) SetRating(r rating.Rater) {
	if s == nil || r == nil {
		return
	}
	s.rater.Store(&raterHolder{rater: r})
}

// raterOrNop never returns nil: before the store exists, and on a node
// that failed to build one, discovery must penalise nobody.
func (s *discoveryService) raterOrNop() rating.Rater {
	if held := s.rater.Load(); held != nil && held.rater != nil {
		return held.rater
	}
	return rating.Nop{}
}

// band reads a peer's standing for a budget decision. A read failure
// leaves the peer at full trust: discovery must not throttle a peer
// because we could not see its record.
func (s *discoveryService) band(id warpnet.WarpPeerID) rating.Band {
	b, err := s.raterOrNop().Band(id)
	if err != nil {
		log.Warnf("discovery: reading standing of %s: %v", id, err)
		return rating.BandTrusted
	}
	return b
}

//goland:noinspection ALL
func NewDiscoveryService(
	ctx context.Context,
	userRepo UserStorer,
	nodeRepo NodeStorer,
	m MetricsOnlineDiscoverer,
) *discoveryService {
	capacity := 32
	leakPerTenSec := 2

	lru := expirable.NewLRU[warpnet.WarpPeerID, warpnet.WarpPeerID](10, nil, time.Hour*24)
	s := &discoveryService{
		ctx:             ctx,
		userRepo:        userRepo,
		nodeRepo:        nodeRepo,
		limiter:         newRateLimiter(capacity, leakPerTenSec),
		discoveryChan:   make(chan discoveredPeer, 128),  //nolint:mnd
		discoveryTicker: time.NewTicker(time.Minute * 5), //nolint:mnd
		stopChan:        make(chan struct{}),
		aliasCache:      lru,
		probed:          newProbedCache(),
		m:               m,
	}
	s.peerLimiter = newPeerLimiter(s.band)
	return s
}

func NewRelayDiscoveryService(ctx context.Context, m MetricsOnlineDiscoverer) *discoveryService {
	lru := expirable.NewLRU[warpnet.WarpPeerID, warpnet.WarpPeerID](4096, nil, time.Hour*72)
	s := &discoveryService{
		ctx:             ctx,
		limiter:         newRateLimiter(32, 2),
		discoveryChan:   make(chan discoveredPeer, 128),  //nolint:mnd
		discoveryTicker: time.NewTicker(time.Minute * 5), //nolint:mnd
		stopChan:        make(chan struct{}),
		aliasCache:      lru,
		probed:          newProbedCache(),
		m:               m,
	}
	s.peerLimiter = newPeerLimiter(s.band)
	return s
}

func newProbedCache() *expirable.LRU[warpnet.WarpPeerID, struct{}] {
	return expirable.NewLRU[warpnet.WarpPeerID, struct{}](4096, nil, probeInterval) //nolint:mnd
}

func (s *discoveryService) Run(n DiscoveryInfoStorer) error {
	if s.discoveryChan == nil {
		return warpnet.WarpError("discovery channel is nil")
	}
	log.Infoln("discovery: service started")

	s.node = n
	s.ownId = s.node.NodeInfo().ID

	asRelay := s.node.NodeInfo().IsRelay()
	asModerator := s.node.NodeInfo().IsModerator()

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.stopChan:
				return
			case <-s.discoveryTicker.C:
				log.Warnf("discovery: stalled")
			case info, ok := <-s.discoveryChan:
				if !ok {
					log.Infoln("discovery: service closed")
					return
				}
				s.discoveryTicker.Reset(time.Minute * 5) //nolint:mnd

				switch {
				case asRelay:
					s.handleAsRelay(info)
				case asModerator:
					s.handleAsModerator(info)
				default:
					s.handleAsMember(info)
				}
			}
		}
	}()
	return nil
}

func (s *discoveryService) DiscoveryHandlerMDNS(pi warpnet.WarpAddrInfo) {
	s.enqueue(pi, sourceMDNS)
}

func (s *discoveryService) DiscoveryHandlerDHT(id warpnet.WarpPeerID) {
	info := warpnet.WarpAddrInfo{ID: id}
	s.enqueue(info, sourceDHT)
}

func (s *discoveryService) DiscoveryHandlerStream(pi warpnet.WarpAddrInfo) {
	if s.node != nil && len(s.node.Peerstore().Addrs(pi.ID)) != 0 {
		return // end discovery loop
	}
	s.enqueue(pi, sourceStream)
}

func (s *discoveryService) DiscoveryHandlerPubSub(pi warpnet.WarpAddrInfo) {
	s.enqueue(pi, sourceGossip) // main source
}

func (s *discoveryService) enqueue(pi warpnet.WarpAddrInfo, source discoverySource) {
	if s == nil || s.discoveryChan == nil {
		log.Errorf("discovery: handle new peer found: nil discovery service")
		return
	}
	log.Debugf("discovery: found peer: %s, source: %s", pi.ID.String(), source)

	if pi.ID == "" || pi.ID == s.ownId {
		return
	}

	// Per-peer first: one chatty gossiper must not be able to spend
	// the whole shared budget and starve discovery of everyone else.
	if !s.peerLimiter.Allow(pi.ID) {
		log.Debugf("discovery: source '%s': peer over its own budget: %s", source, pi.ID.String())
		if err := s.raterOrNop().Record(pi.ID, rating.KindDiscoveryFlood); err != nil {
			log.Warnf("discovery: rating flood by %s: %v", pi.ID, err)
		}
		return
	}
	if !s.limiter.Allow() {
		log.Infof("discovery: source '%s': limited by rate limiter: %s", source, pi.ID.String())
		return
	}

	select {
	case <-s.stopChan:
		return
	case s.discoveryChan <- discoveredPeer{
		ID:     pi.ID,
		Addrs:  pi.Addrs,
		Source: source,
	}:
	default:
		div := cap(s.discoveryChan) / 10
		jitter := rand.IntN(div) //#nosec
		dropMessagesNum := jitter + 1
		log.Warnf("discovery: channel overflow %d, drop %d first messages", cap(s.discoveryChan), dropMessagesNum)
		for range dropMessagesNum {
			<-s.discoveryChan // drop old data
		}
	}
}

func (s *discoveryService) handleAsMember(peer discoveredPeer) {
	if s == nil || s.node == nil || s.nodeRepo == nil || s.userRepo == nil {
		log.Errorf("discovery: handle: nil discovery service")
		return
	}

	if s.nodeRepo.IsBlocklisted(peer.ID.String()) {
		log.Infof("discovery: source '%s': found blocklisted peer: %s", peer.Source, peer.ID.String())
		s.m.PushStatusOffline(peer.ID.String())
		return
	}

	pi := warpnet.WarpAddrInfo{ID: peer.ID, Addrs: peer.Addrs}

	err := s.connect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: source '%s': connecting is backoffed: %s", peer.Source, pi.ID)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}
	if err != nil {
		log.Debugf(
			"discovery: source '%s': failed to connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err,
		)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Warnf(
			"discovery: source '%s': failed to connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err)
		s.m.PushStatusOffline(pi.ID.String())
		s.recordDialFailure(pi)
		return
	}

	if s.aliasCache.Contains(peer.ID) {
		log.Infof("discovery: source '%s': found alias peer: %s", peer.Source, peer.ID.String())
		s.m.PushStatusOnline(pi.ID.String())
		s.node.SetMaxNodePriority(pi.ID)
		return
	}

	// Every rediscovery of a peer used to cost a full info round trip,
	// so republished gossip alone produced O(N²) requests across the
	// network. Ask at most once per probeInterval.
	if !s.shouldProbe(peer.ID) {
		log.Debugf("discovery: source '%s': already probed recently: %s", peer.Source, pi.ID.String())
		s.m.PushStatusOnline(pi.ID.String())
		return
	}

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Warnf(
			"discovery: source '%s': request node %s info: %s",
			peer.Source, pi.ID.String(), err.Error(),
		)
		return
	}

	for _, alias := range info.Aliases {
		s.aliasCache.Add(alias, pi.ID)
	}

	s.node.SetNodePriority(pi.ID, info.Reachability)

	if info.IsRelay() {
		return
	}
	if pi.ID.String() == mastodon.GatewayNodeID() {
		return
	}

	s.m.PushStatusOnline(pi.ID.String())

	if info.IsModerator() {
		return
	}

	existedUser, err := s.userRepo.GetByNodeID(pi.ID.String())
	if !errors.Is(err, database.ErrUserNotFound) && !existedUser.IsOffline {
		return
	}

	fmt.Printf("\033[1mdiscovery: connected to new peer: %s, source '%s' \033[0m\n", pi.String(), peer.Source)

	user, err := s.requestNodeUser(pi, info.OwnerId)
	if err != nil {
		log.Errorf("discovery: source '%s': request node user: %s", peer.Source, err.Error())
		return
	}

	newUser, err := s.userRepo.Create(user)
	if errors.Is(err, database.ErrUserAlreadyExists) {
		newUser, _ = s.userRepo.Update(user.Id, user) //nolint:wastedassign
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': create user %s from new peer: %v, ",
			peer.Source, user.Id, err)
		return
	}
	log.Infof(
		"discovery: new user added: id: %s, name: %s, node_id: %s, created_at: %s, RTT: %d, source: %s",
		newUser.Id,
		newUser.Username,
		newUser.NodeId,
		newUser.CreatedAt,
		newUser.RoundTripTime,
		peer.Source,
	)
}

func (s *discoveryService) handleAsRelay(peer discoveredPeer) {
	if s == nil || s.node == nil {
		log.Errorf("discovery: relay handle: nil discovery service")
		return
	}

	if peer.ID == "" || peer.ID == s.ownId {
		return
	}

	pi := warpnet.WarpAddrInfo{ID: peer.ID, Addrs: peer.Addrs}

	err := s.connect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: source '%s': relay handle: connecting is backoffed: %s", peer.Source, pi.ID)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}
	if err != nil {
		log.Debugf(
			"discovery: source '%s': relay handle: connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err,
		)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Warnf(
			"discovery: source '%s': relay handle: connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err,
		)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}

	if s.aliasCache.Contains(peer.ID) {
		log.Debugf("discovery: source '%s': found alias peer: %s", peer.Source, peer.ID.String())
		s.m.PushStatusOnline(pi.ID.String())
		return
	}

	s.m.PushStatusOnline(pi.ID.String())

	if !s.shouldProbe(peer.ID) {
		return
	}

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Warnf("discovery: source '%s': request node info: %s", peer.Source, err.Error())
		return
	}
	s.node.SetNodePriority(pi.ID, info.Reachability)

	for _, alias := range info.Aliases {
		s.aliasCache.Add(alias, pi.ID)
	}
}

func (s *discoveryService) handleAsModerator(pi discoveredPeer) {
	log.Infof("discovery: id %s, addrs %v, source '%s'", pi.ID.String(), pi.Addrs, pi.Source)
}

// recordDialFailure charges a peer for a dial that actually reached
// for it and failed.
//
// A failure with no address to dial is *our* gap, not the peer's: it
// means gossip named a peer our routing table cannot resolve yet, which
// happens constantly and harmlessly while a node is still finding its
// feet. Charging it made honest nodes rate each other down during
// ordinary discovery — observed in a live three-node run before this
// guard existed.
func (s *discoveryService) recordDialFailure(pi warpnet.WarpAddrInfo) {
	if s == nil {
		return
	}
	known := len(pi.Addrs) > 0
	if !known && s.node != nil && s.node.Peerstore() != nil {
		known = len(s.node.Peerstore().Addrs(pi.ID)) > 0
	}
	if !known {
		return
	}
	if err := s.raterOrNop().Record(pi.ID, rating.KindDialFailure); err != nil {
		log.Warnf("discovery: rating dial failure of %s: %v", pi.ID, err)
	}
}

// shouldProbe reports whether this peer may be asked for its info now,
// and marks it probed if so.
func (s *discoveryService) shouldProbe(id warpnet.WarpPeerID) bool {
	if s == nil || s.probed == nil {
		return true
	}
	if s.probed.Contains(id) {
		return false
	}
	s.probed.Add(id, struct{}{})
	return true
}

// connect dials through the node's backoff when it offers one. The
// discovery loop used to call SimpleConnect, the raw host dial, which
// skipped the backoff entirely — so a dead peer that gossip kept
// republishing was redialled forever.
func (s *discoveryService) connect(pi warpnet.WarpAddrInfo) error {
	if backoffer, ok := s.node.(BackoffConnector); ok {
		return backoffer.Connect(pi)
	}
	return s.node.SimpleConnect(pi)
}

const errPeerRejectedInfo = warpnet.WarpError("peer rejected info request")

func (s *discoveryService) requestNodeInfo(pi warpnet.WarpAddrInfo) (info warpnet.NodeInfo, err error) {
	infoResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_INFO, nil)
	if err != nil {
		return info, fmt.Errorf("failed to get info from new peer %s: %w", pi.ID.String(), err)
	}

	if len(infoResp) == 0 {
		err := warpnet.WarpError("no info response from new peer")
		return info, fmt.Errorf("%w: %s", err, pi.ID.String())
	}

	var possibleError event.ResponseError
	if _ = json.Unmarshal(infoResp, &possibleError); possibleError.Message != "" {
		return info, fmt.Errorf("%w: %s: %s", errPeerRejectedInfo, pi.ID.String(), possibleError.Message)
	}
	// back compat: older nodes replied to middleware failures with a bare
	// JSON array of messages instead of event.ResponseError.
	var legacyError []string
	if _ = json.Unmarshal(infoResp, &legacyError); len(legacyError) != 0 {
		return info, fmt.Errorf(
			"%w: %s: %s", errPeerRejectedInfo, pi.ID.String(), strings.Join(legacyError, "; "),
		)
	}

	err = json.Unmarshal(infoResp, &info)
	if err != nil {
		return info, fmt.Errorf("failed to unmarshal info from new peer: %s %w", infoResp, err)
	}
	if info.OwnerId == "" {
		err := warpnet.WarpError("node info has no owner")
		return info, fmt.Errorf("%w: %s", err, pi.ID.String())
	}
	return info, nil
}

func (s *discoveryService) requestNodeUser(pi warpnet.WarpAddrInfo, userId string) (user domain.User, err error) {
	if userId == "" {
		return user, warpnet.WarpError("empty user id")
	}

	getUserEvent := event.GetUserEvent{UserId: userId}

	now := time.Now()
	userResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_USER, getUserEvent)
	if err != nil {
		return user, fmt.Errorf("failed to user data from new peer %s: %w", pi.ID.String(), err)
	}
	elapsed := time.Since(now)

	if len(userResp) == 0 {
		err := warpnet.WarpError("no user response from new peer")
		return user, fmt.Errorf("%w: %s", err, pi.String())
	}

	err = json.Unmarshal(userResp, &user)
	if err != nil {
		return user, fmt.Errorf("failed to unmarshal user from new peer: %w", err)
	}

	user.IsOffline = false
	user.NodeId = pi.ID.String()
	user.RoundTripTime = elapsed.Milliseconds()
	return user, nil
}

func (s *discoveryService) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("discovery: close recovered from panic: %v", r)
		}
	}()
	if s.stopChan == nil {
		return
	}
	s.discoveryTicker.Stop()
	s.peerLimiter.Close()
	close(s.stopChan)
	close(s.discoveryChan)
	log.Infoln("discovery: closed")
}
