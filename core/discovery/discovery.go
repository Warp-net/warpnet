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
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/core/backoff"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p/core/crypto/pb"
	log "github.com/sirupsen/logrus"
)

type DiscoveryHandler func(warpnet.WarpAddrInfo)

type DiscoveryInfoStorer interface {
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
	SimpleConnect(warpnet.WarpAddrInfo) error
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
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

	ownId   warpnet.WarpPeerID
	limiter *leakyBucketRateLimiter
	cache   *discoveryCache
	retrier retrier.Retrier

	// channel is needed to collect discoveries while node is setting up
	discoveryChan   chan discoveredPeer
	discoveryTicker *time.Ticker
	stopChan        chan struct{}
}

//goland:noinspection ALL
func NewDiscoveryService(
	ctx context.Context,
	userRepo UserStorer,
	nodeRepo NodeStorer,
) *discoveryService {
	capacity := 32
	leakPerTenSec := 2
	return &discoveryService{
		ctx:             ctx,
		node:            nil,
		retrier:         retrier.New(time.Second, 3, retrier.ExponentialBackoff),
		userRepo:        userRepo,
		nodeRepo:        nodeRepo,
		limiter:         newRateLimiter(capacity, leakPerTenSec),
		cache:           newDiscoveryCache(),
		discoveryChan:   make(chan discoveredPeer, 128),  //nolint:mnd
		discoveryTicker: time.NewTicker(time.Minute * 5), //nolint:mnd
		stopChan:        make(chan struct{}),
	}
}

func NewBootstrapDiscoveryService(ctx context.Context) *discoveryService {
	return NewDiscoveryService(ctx, nil, nil)
}

func (s *discoveryService) Run(n DiscoveryInfoStorer) error {
	if s == nil {
		return warpnet.WarpError("nil discovery service")
	}
	if s.discoveryChan == nil {
		return warpnet.WarpError("discovery channel is nil")
	}
	log.Infoln("discovery: service started")

	s.node = n
	s.ownId = s.node.NodeInfo().ID

	isBootstrap := s.node.NodeInfo().IsBootstrap()
	isModerator := s.node.NodeInfo().IsModerator()

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
				case isBootstrap:
					s.handleAsBootstrap(info)
				case isModerator:
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
		div := int(cap(s.discoveryChan) / 10) //nolint:mnd
		jitter := rand.IntN(div)
		dropMessagesNum := jitter + 1
		log.Warnf("discovery: channel overflow %d, drop %d first messages", cap(s.discoveryChan), dropMessagesNum)
		for i := 0; i < dropMessagesNum; i++ {
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
		return
	}

	pi := warpnet.WarpAddrInfo{ID: peer.ID, Addrs: peer.Addrs}

	err := s.node.SimpleConnect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: source '%s': connecting is backoffed: %s", peer.Source, pi.ID)
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
		return
	}

	err = s.requestChallenge(pi)
	if errors.Is(err, ErrChallengeMismatch) || errors.Is(err, ErrChallengeSignatureInvalid) {
		log.Warnf("discovery: source '%s': challenge is invalid for peer: %s", peer.Source, pi.ID.String())
		_ = s.nodeRepo.Blocklist(pi.ID.String())
		s.node.Peerstore().RemovePeer(pi.ID)
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': failed to request challenge for peer %s: %v",
			peer.Source, pi.ID, err)
		return
	}

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Errorf("discovery: source '%s': request node info: %s", peer.Source, err.Error())
		return
	}

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
		newUser, _ = s.userRepo.Update(user.Id, user)
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': create user %s from new peer: %v, ",
			peer.Source, user.Id, err)
		return
	}
	log.Infof(
		"discovery: new user added: id: %s, name: %s, node_id: %s, created_at: %s, latency: %d, source: %s",
		newUser.Id,
		newUser.Username,
		newUser.NodeId,
		newUser.CreatedAt,
		newUser.Latency,
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
		return
	}

	log.Infof("node challenge request: %s, source '%s'", pi.ID.String(), peer.Source)

	err = s.requestChallenge(pi)
	if errors.Is(err, ErrChallengeMismatch) || errors.Is(err, ErrChallengeSignatureInvalid) {
		log.Warnf(
			"discovery: source '%s': bootstrap handle: challenge is invalid for peer: %s",
			pi.ID.String(), peer.Source,
		)
		s.node.Peerstore().RemovePeer(pi.ID)
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: source '%s': bootstrap handle: request challenge for peer %s: %v",
			peer.Source, pi.ID, err,
		)
		return
	}
}

func (s *discoveryService) handleAsModerator(pi discoveredPeer) {
	log.Infof("discovery: id %s, addrs %v, source '%s'", pi.ID.String(), pi.Addrs, pi.Source)
}

const (
	ErrChallengeMismatch         warpnet.WarpError = "challenge mismatch"
	ErrChallengeSignatureInvalid warpnet.WarpError = "invalid challenge signature"
)

func (s *discoveryService) requestChallenge(pi warpnet.WarpAddrInfo) error {
	if s == nil {
		return errors.New("nil discovery service")
	}
	if s.cache.IsChallengedAlready(pi.ID) {
		log.Debugf("discovery: peer %s already challenged", pi.ID.String())
		return nil
	}

	var level int
	if s.nodeRepo != nil {
		term, err := s.nodeRepo.BlocklistTerm(pi.ID.String())
		if err != nil {
			log.Errorf("discovery: peer %s blocklist term: %s", pi.ID.String(), err.Error())
		}

		if term != nil {
			level = int(term.Level)
		}
	}

	ownChallenges, samples, err := s.composeChallengeRequest(level)
	if err != nil {
		return err
	}

	var resp []byte
	err = s.retrier.Try(s.ctx, func() error {
		resp, err = s.node.GenericStream(
			pi.ID.String(),
			event.PUBLIC_POST_NODE_CHALLENGE,
			event.ChallengeEvent{Samples: samples},
		)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to get challenge from new peer %s: %v", pi.ID.String(), err)
	}

	if resp == nil || len(resp) == 0 {
		return fmt.Errorf("no challenge response from new peer %s", pi.ID.String())
	}

	var challengeResp event.ChallengeResponse
	err = json.Unmarshal(resp, &challengeResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal challenge from new peer: %s %v", resp, err)
	}

	if err := s.validateChallenges(ownChallenges, pi.ID, challengeResp.Solutions); err != nil {
		return err
	}

	s.cache.SetAsChallenged(pi.ID)
	return nil
}

type challenge = []byte

func (s *discoveryService) composeChallengeRequest(challengeLevel int) ([]challenge, []event.ChallengeSample, error) {
	if challengeLevel == 0 {
		challengeLevel = 1
	}

	ownChalenges := make([][]byte, challengeLevel)
	samples := make([]event.ChallengeSample, challengeLevel)
	for i := 0; i < challengeLevel; i++ {
		nonce := rand.Int64()
		ownChallenge, location, err := security.GenerateChallenge(root.GetCodeBase(), nonce)
		if err != nil {
			return nil, nil, err
		}
		ownChalenges[i] = ownChallenge
		samples[i] = event.ChallengeSample{
			DirStack:  location.DirStack,
			FileStack: location.FileStack,
			Nonce:     nonce,
		}
	}
	return ownChalenges, samples, nil
}

func (s *discoveryService) validateChallenges(
	ownChallenges []challenge,
	peerId warpnet.WarpPeerID,
	solutions []event.ChallengeSolution) error {
	if len(solutions) != len(ownChallenges) {
		return fmt.Errorf("invalid number of solutions: %d != %d", len(solutions), len(ownChallenges))
	}

	for i := range solutions {
		ownChallenge := ownChallenges[i]
		solution := solutions[i]

		challengeRespDecoded, err := hex.DecodeString(solution.Challenge)
		if err != nil {
			return fmt.Errorf("failed to decode challenge origin: %v", err)
		}

		if !bytes.Equal(ownChallenge, challengeRespDecoded) {
			log.Errorf("challenge mismatch: %s != %s", hex.EncodeToString(ownChallenge), solution.Challenge)
			return ErrChallengeMismatch
		}

		peerstorePubKey := s.node.Peerstore().PubKey(peerId)
		if peerstorePubKey == nil {
			return fmt.Errorf("peer %s has no public key", peerId.String())
		}
		if peerstorePubKey.Type() != pb.KeyType_Ed25519 {
			return warpnet.WarpError("peer is not an Ed25519 public key")
		}

		rawPubKey, _ := peerstorePubKey.Raw()
		if err := security.VerifySignature(rawPubKey, challengeRespDecoded, solution.Signature); err != nil {
			log.Errorf("invalid signature: %v", err)
			return ErrChallengeSignatureInvalid
		}
	}
	return nil
}

func (s *discoveryService) requestNodeInfo(pi warpnet.WarpAddrInfo) (info warpnet.NodeInfo, err error) {
	if s == nil {
		return info, errors.New("nil discovery service")
	}

	infoResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_INFO, nil)
	if err != nil {
		return info, fmt.Errorf("failed to get info from new peer %s: %v", pi.ID.String(), err)
	}

	if infoResp == nil || len(infoResp) == 0 {
		return info, fmt.Errorf("no info response from new peer %s", pi.ID.String())
	}

	err = json.Unmarshal(infoResp, &info)
	if err != nil {
		return info, fmt.Errorf("failed to unmarshal info from new peer: %s %v", infoResp, err)
	}
	if info.OwnerId == "" {
		return info, fmt.Errorf("node info %s has no owner", pi.ID.String())
	}
	return info, nil
}

func (s *discoveryService) requestNodeUser(pi warpnet.WarpAddrInfo, userId string) (user domain.User, err error) {
	if s == nil {
		return user, errors.New("nil discovery service")
	}
	if userId == "" {
		return user, errors.New("empty user id")
	}

	getUserEvent := event.GetUserEvent{UserId: userId}

	userResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_USER, getUserEvent)
	if err != nil {
		return user, fmt.Errorf("failed to user data from new peer %s: %v", pi.ID.String(), err)
	}

	if userResp == nil || len(userResp) == 0 {
		return user, fmt.Errorf("no user response from new peer %s", pi.String())
	}

	err = json.Unmarshal(userResp, &user)
	if err != nil {
		return user, fmt.Errorf("failed to unmarshal user from new peer: %v", err)
	}

	user.IsOffline = false
	user.NodeId = pi.ID.String()
	user.Latency = int64(s.node.Peerstore().LatencyEWMA(pi.ID))
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
	close(s.stopChan)
	close(s.discoveryChan)
	log.Infoln("discovery: closed")
}
