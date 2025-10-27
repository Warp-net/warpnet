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
	BlocklistRemove(peerId warpnet.WarpPeerID)
	IsBlocklisted(peerId warpnet.WarpPeerID) (bool, database.BlockLevel)
	Blocklist(peerId warpnet.WarpPeerID) error
}

type UserStorer interface {
	Create(user domain.User) (domain.User, error)
	Update(userId string, newUser domain.User) (domain.User, error)
	GetByNodeID(nodeID string) (user domain.User, err error)
}

type discoveryService struct {
	ctx      context.Context
	node     DiscoveryInfoStorer
	userRepo UserStorer
	nodeRepo NodeStorer

	ownId   warpnet.WarpPeerID
	limiter *leakyBucketRateLimiter
	cache   *discoveryCache

	// channel is needed to collect discoveries while node is setting up
	discoveryChan   chan warpnet.WarpAddrInfo
	discoveryTicker *time.Ticker
	stopChan        chan struct{}
}

//goland:noinspection ALL
func NewDiscoveryService(
	ctx context.Context,
	userRepo UserStorer,
	nodeRepo NodeStorer,
) *discoveryService {
	capacity := 16
	leakPerSec := 1
	return &discoveryService{
		ctx:             ctx,
		node:            nil,
		userRepo:        userRepo,
		nodeRepo:        nodeRepo,
		limiter:         newRateLimiter(capacity, leakPerSec),
		cache:           newDiscoveryCache(),
		discoveryChan:   make(chan warpnet.WarpAddrInfo, 1000),
		discoveryTicker: time.NewTicker(time.Minute * 5),
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
				s.discoveryTicker.Reset(time.Minute * 5)

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

const dropMessagesLimit = 5

func (s *discoveryService) HandlePeerFound(pi warpnet.WarpAddrInfo) {
	if s == nil || s.discoveryChan == nil {
		log.Errorf("discovery: handle peer found: nil discovery service")
		return
	}

	select {
	case s.discoveryChan <- pi:
	default:
		log.Warnf("discovery: channel overflow %d", cap(s.discoveryChan))
		for i := 0; i < dropMessagesLimit; i++ {
			<-s.discoveryChan // drop old data
		}
	}
}

type pubsubDiscoveryMessage struct {
	Body []warpnet.WarpAddrInfo `json:"body"`
}

func (s *discoveryService) PubSubDiscoveryHandler() func([]byte) error {
	return func(data []byte) error {
		if s == nil || s.discoveryChan == nil {
			log.Errorf("discovery: pubsub: nil discovery service")
			return nil
		}

		if len(data) == 0 {
			return nil
		}

		var msg pubsubDiscoveryMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("discovery: pubsub: unmarshal pubsub message: %v %s", err, data)
		}

		if len(msg.Body) == 0 {
			return fmt.Errorf("discovery: pubsub: empty message: %s", string(data))
		}

		for _, info := range msg.Body {
			if info.ID == s.ownId {
				continue
			}

			select {
			case s.discoveryChan <- info:
			default:
				log.Warnf("discovery: channel overflow %d", cap(s.discoveryChan))
				for i := 0; i < dropMessagesLimit; i++ {
					<-s.discoveryChan // drop old data
				}
			}
		}
		return nil
	}
}

func (s *discoveryService) handleAsMember(pi warpnet.WarpAddrInfo) {
	if s == nil || s.node == nil || s.nodeRepo == nil || s.userRepo == nil {
		log.Errorf("discovery: handle: nil discovery service")
		return
	}

	if pi.ID == "" || pi.ID == s.ownId {
		return
	}

	if !s.limiter.Allow() {
		log.Infof("discovery: limited by rate limiter: %s", pi.ID.String())
		return
	}

	ok, _ := s.nodeRepo.IsBlocklisted(pi.ID)
	if ok {
		log.Infof("discovery: found blocklisted peer: %s", pi.ID.String())
		return
	}

	err := s.node.SimpleConnect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: connecting is backoffed: %s", pi.ID)
		return
	}
	if err != nil {
		log.Debugf("discovery: failed to connect to new peer %s: %v", pi.ID.String(), err)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Warnf("discovery: failed to connect to new peer %s: %v", pi.ID.String(), err)
		return
	}

	err = s.requestChallenge(pi)
	if errors.Is(err, ErrChallengeMismatch) || errors.Is(err, ErrChallengeSignatureInvalid) {
		log.Warnf("discovery: challenge is invalid for peer: %s\n", pi.ID.String())
		_ = s.nodeRepo.Blocklist(pi.ID)
		s.node.Peerstore().RemovePeer(pi.ID)
		return
	}
	if err != nil {
		log.Errorf("discovery: failed to request challenge for peer %s: %v\n", pi.ID, err)
		return
	}

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Errorf("discovery: request node info: %s", err.Error())
		return
	}

	if info.IsBootstrap() || info.IsModerator() {
		return
	}

	existedUser, err := s.userRepo.GetByNodeID(pi.ID.String())
	if !errors.Is(err, database.ErrUserNotFound) && !existedUser.IsOffline {
		return
	}

	fmt.Printf("\033[1mdiscovery: connected to new peer: %s \033[0m\n", pi.String())

	user, err := s.requestNodeUser(pi, info.OwnerId)
	if err != nil {
		log.Errorf("discovery: request node user: %s", err.Error())
		return
	}

	newUser, err := s.userRepo.Create(user)
	if errors.Is(err, database.ErrUserAlreadyExists) {
		newUser, _ = s.userRepo.Update(user.Id, user)
		return
	}
	if err != nil {
		log.Errorf("discovery: create user from new peer: %s, user id %s", err, user.Id)
		return
	}
	log.Infof(
		"discovery: new user added: id: %s, name: %s, node_id: %s, created_at: %s, latency: %d",
		newUser.Id,
		newUser.Username,
		newUser.NodeId,
		newUser.CreatedAt,
		newUser.Latency,
	)
}

func (s *discoveryService) handleAsBootstrap(pi warpnet.WarpAddrInfo) {
	if s == nil || s.node == nil {
		log.Errorf("discovery: bootstrap handle: nil discovery service")
		return
	}

	if pi.ID == "" || pi.ID == s.ownId {
		return
	}

	err := s.node.SimpleConnect(pi)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		log.Debugf("discovery: bootstrap handle: connecting is backoffed: %s", pi.ID)
		return
	}
	if err != nil {
		log.Debugf("discovery: bootstrap handle: connect to new peer %s: %v", pi.ID.String(), err)
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Warnf("discovery: bootstrap handle: connect to new peer %s: %v", pi.ID.String(), err)
		return
	}

	log.Infof("node challenge request: %s", pi.ID.String())

	err = s.requestChallenge(pi)
	if errors.Is(err, ErrChallengeMismatch) || errors.Is(err, ErrChallengeSignatureInvalid) {
		log.Warnf("discovery: bootstrap handle: challenge is invalid for peer: %s\n", pi.ID.String())
		s.node.Peerstore().RemovePeer(pi.ID)
		return
	}
	if err != nil {
		log.Errorf("discovery: bootstrap handle: request challenge for peer %s: %v\n", pi.ID, err)
		return
	}
}

func (s *discoveryService) handleAsModerator(pi warpnet.WarpAddrInfo) {}

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

	nonce := rand.Int64()
	ownChallenge, location, err := security.GenerateChallenge(root.GetCodeBase(), nonce)
	if err != nil {
		return err
	}

	resp, err := s.node.GenericStream(
		pi.ID.String(),
		event.PUBLIC_POST_NODE_CHALLENGE,
		event.ChallengeEvent{
			DirStack:  location.DirStack,
			FileStack: location.FileStack,
			Nonce:     nonce,
		},
	)
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

	challengeRespDecoded, err := hex.DecodeString(challengeResp.Challenge)
	if err != nil {
		return fmt.Errorf("failed to decode challenge origin: %v", err)
	}

	if !bytes.Equal(ownChallenge, challengeRespDecoded) {
		log.Errorf("discovery: challenge mismatch: %s != %s", hex.EncodeToString(ownChallenge), challengeResp.Challenge)
		return ErrChallengeMismatch
	} else {
		log.Debugf("discovery: challenge match: %s == %s", hex.EncodeToString(ownChallenge), challengeResp.Challenge)
	}

	peerstorePubKey := s.node.Peerstore().PubKey(pi.ID)
	if peerstorePubKey == nil {
		return fmt.Errorf("peer %s has no public key", pi.ID.String())
	}
	if peerstorePubKey.Type() != pb.KeyType_Ed25519 {
		return errors.New("peer is not an Ed25519 public key")
	}

	rawPubKey, _ := peerstorePubKey.Raw()
	if err := security.VerifySignature(rawPubKey, challengeRespDecoded, challengeResp.Signature); err != nil {
		log.Errorf("invalid signature: %v", err)
		return ErrChallengeSignatureInvalid
	}

	s.cache.SetAsChallenged(pi.ID)
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
}
