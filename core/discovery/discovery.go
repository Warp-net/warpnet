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
	"github.com/Warp-net/warpnet/core/challenge"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"math/rand/v2"
	"time"

	"github.com/Warp-net/warpnet/core/backoff"
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

type DiscoveryChallenger interface {
	Challenge(pubKey ic.PubKey, level int, streamF challenge.StreamFunc) (bool, error)
}

type NodeStorer interface {
	BlocklistRemove(peerId string) error
	IsBlocklisted(peerId string) bool
	Blocklist(peerId string) error
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

	ownId      warpnet.WarpPeerID
	limiter    *leakyBucketRateLimiter
	challenger DiscoveryChallenger

	// channel is needed to collect discoveries while node is setting up
	discoveryChan   chan discoveredPeer
	discoveryTicker *time.Ticker
	stopChan        chan struct{}

	m MetricsOnlineDiscoverer
}

//goland:noinspection ALL
func NewDiscoveryService(
	ctx context.Context,
	userRepo UserStorer,
	nodeRepo NodeStorer,
	challenger DiscoveryChallenger,
	m MetricsOnlineDiscoverer,
) *discoveryService {
	capacity := 32
	leakPerTenSec := 2
	return &discoveryService{
		ctx:             ctx,
		userRepo:        userRepo,
		nodeRepo:        nodeRepo,
		limiter:         newRateLimiter(capacity, leakPerTenSec),
		challenger:      challenger,
		discoveryChan:   make(chan discoveredPeer, 128),  //nolint:mnd
		discoveryTicker: time.NewTicker(time.Minute * 5), //nolint:mnd
		stopChan:        make(chan struct{}),
		m:               m,
	}
}

func NewBootstrapDiscoveryService(
	ctx context.Context, challenger DiscoveryChallenger, m MetricsOnlineDiscoverer) *discoveryService {
	return &discoveryService{
		ctx:             ctx,
		limiter:         newRateLimiter(32, 2),
		challenger:      challenger,
		discoveryChan:   make(chan discoveredPeer, 128),  //nolint:mnd
		discoveryTicker: time.NewTicker(time.Minute * 5), //nolint:mnd
		stopChan:        make(chan struct{}),
		m:               m,
	}
}

func (s *discoveryService) Run(n DiscoveryInfoStorer) error {
	if s.discoveryChan == nil {
		return warpnet.WarpError("discovery channel is nil")
	}
	log.Infoln("discovery: service started")

	s.node = n
	s.ownId = s.node.NodeInfo().ID

	asBootstrap := s.node.NodeInfo().IsBootstrap()
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
				case asBootstrap:
					s.handleAsBootstrap(info)
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

	if !s.limiter.Allow() {
		log.Infof("discovery: source '%s': limited by rate limiter: %s", source, pi.ID.String())
		return
	}

	select {
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

	err := s.node.SimpleConnect(pi)
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
		return
	}

	isRepeatable, err := s.challenger.Challenge(
		s.node.Peerstore().PubKey(pi.ID),
		s.getChallengeLevel(pi.ID),
		s.requestChallenge(pi.ID),
	)
	if errors.Is(err, challenge.ErrChallengeMismatch) || errors.Is(err, challenge.ErrChallengeSignatureInvalid) {
		log.Warnf("discovery: source '%s': challenge is invalid for peer: %s", peer.Source, pi.ID.String())
		if isRepeatable {
			_ = s.nodeRepo.Blocklist(pi.ID.String())
		} else {
			// reset block time
			_ = s.nodeRepo.BlocklistRemove(pi.ID.String())
			_ = s.nodeRepo.Blocklist(pi.ID.String())
		}
		s.node.Peerstore().RemovePeer(pi.ID)
		s.node.SetMinNodePriority(pi.ID)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': failed to request challenge for peer %s: %v",
			peer.Source, pi.ID, err)
		return
	}

	s.m.PushStatusOnline(pi.ID.String())

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Errorf("discovery: source '%s': request node info: %s", peer.Source, err.Error())
		return
	}

	s.node.SetNodePriority(pi.ID, info.Reachability)

	if info.IsBootstrap() || info.IsModerator() {
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

func (s *discoveryService) handleAsBootstrap(peer discoveredPeer) {
	if s == nil || s.node == nil {
		log.Errorf("discovery: bootstrap handle: nil discovery service")
		return
	}

	if peer.ID == "" || peer.ID == s.ownId {
		return
	}

	pi := warpnet.WarpAddrInfo{ID: peer.ID, Addrs: peer.Addrs}

	err := s.node.SimpleConnect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: source '%s': bootstrap handle: connecting is backoffed: %s", peer.Source, pi.ID)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}
	if err != nil {
		log.Debugf(
			"discovery: source '%s': bootstrap handle: connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err,
		)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Warnf(
			"discovery: source '%s': bootstrap handle: connect to new peer %s: %v",
			peer.Source, pi.ID.String(), err,
		)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}

	_, err = s.challenger.Challenge(
		s.node.Peerstore().PubKey(pi.ID),
		s.getChallengeLevel(pi.ID),
		s.requestChallenge(pi.ID),
	)
	if errors.Is(err, challenge.ErrChallengeMismatch) || errors.Is(err, challenge.ErrChallengeSignatureInvalid) {
		log.Warnf(
			"discovery: source '%s': bootstrap handle: challenge is invalid for peer: %s",
			peer.Source, pi.ID.String(),
		)
		s.node.Peerstore().RemovePeer(pi.ID)
		s.node.SetMinNodePriority(pi.ID)
		s.m.PushStatusOffline(pi.ID.String())
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': bootstrap handle: request challenge for peer %s: %v",
			peer.Source, pi.ID, err,
		)
		return
	}
	s.m.PushStatusOnline(pi.ID.String())

	if rand.IntN(10)/3 == 0 { //nolint:gosec
		info, err := s.requestNodeInfo(pi)
		if err != nil {
			log.Errorf("discovery: source '%s': request node info: %s", peer.Source, err.Error())
			return
		}
		s.node.SetNodePriority(pi.ID, info.Reachability)
	}
}

func (s *discoveryService) handleAsModerator(pi discoveredPeer) {
	log.Infof("discovery: id %s, addrs %v, source '%s'", pi.ID.String(), pi.Addrs, pi.Source)
}

func (s *discoveryService) requestChallenge(peerId warpnet.WarpPeerID) challenge.StreamFunc {
	return func(coord []event.ChallengeSample) ([]event.ChallengeSolution, error) {
		resp, err := s.node.GenericStream(
			peerId.String(),
			event.PUBLIC_POST_NODE_CHALLENGE,
			event.ChallengeEvent{Coordinates: coord},
		)
		if err != nil {
			return nil, err
		}
		if len(resp) == 0 {
			err := warpnet.WarpError("no challenge response from new peer")
			return nil, fmt.Errorf("%w: %s", err, peerId.String())
		}

		var challengeResp event.ChallengeResponse
		err = json.Unmarshal(resp, &challengeResp)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal challenge from new peer: %s %w", resp, err)
		}
		return challengeResp.Solutions, nil
	}
}

func (s *discoveryService) getChallengeLevel(peerId warpnet.WarpPeerID) (level int) {
	if s.nodeRepo == nil {
		return 1
	}
	term, err := s.nodeRepo.BlocklistTerm(peerId.String())
	if err != nil {
		log.Errorf("discovery: peer %s blocklist term: %s", peerId.String(), err.Error())
		return 1
	}
	if term != nil {
		return int(term.Level)
	}
	return 1
}

func (s *discoveryService) requestNodeInfo(pi warpnet.WarpAddrInfo) (info warpnet.NodeInfo, err error) {
	infoResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_INFO, nil)
	if err != nil {
		return info, fmt.Errorf("failed to get info from new peer %s: %w", pi.ID.String(), err)
	}

	if len(infoResp) == 0 {
		err := warpnet.WarpError("no info response from new peer")
		return info, fmt.Errorf("%w: %s", err, pi.ID.String())
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

func (s *discoveryService) getBlockLevel(pi warpnet.WarpAddrInfo) (level int) {
	if s.nodeRepo == nil {
		return 1
	}
	term, err := s.nodeRepo.BlocklistTerm(pi.ID.String())
	if err != nil {
		log.Errorf("discovery: peer %s blocklist term: %s", pi.ID.String(), err.Error())
		return 1
	}
	if term != nil {
		level = int(term.Level)
	}
	return level
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
	close(s.stopChan)
	close(s.discoveryChan)
	log.Infoln("discovery: closed")
}
