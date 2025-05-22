/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package discovery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
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
	"math/rand/v2"
	"sync/atomic"
	"time"
)

type DiscoveryHandler func(warpnet.PeerAddrInfo)

type DiscoveryInfoStorer interface {
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
	Mux() warpnet.WarpProtocolSwitch
	Network() warpnet.WarpNetwork
	Connect(p warpnet.PeerAddrInfo) error
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

type NodeStorer interface {
	BlocklistRemove(ctx context.Context, peerId warpnet.WarpPeerID) (err error)
	IsBlocklisted(ctx context.Context, peerId warpnet.WarpPeerID) (bool, error)
	Blocklist24h(ctx context.Context, peerId warpnet.WarpPeerID) error
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
	version  *semver.Version
	pskHash  string

	handlers []DiscoveryHandler

	retrier        retrier.Retrier
	limiter        *leakyBucketRateLimiter
	cache          *discoveryCache
	bootstrapAddrs []warpnet.PeerAddrInfo

	discoveryChan chan warpnet.PeerAddrInfo
	stopChan      chan struct{}
	syncDone      *atomic.Bool
}

//goland:noinspection ALL
func NewDiscoveryService(
	ctx context.Context,
	userRepo UserStorer,
	nodeRepo NodeStorer,
	handlers ...DiscoveryHandler,
) *discoveryService {
	addrInfos, _ := config.Config().Node.AddrInfos()

	return &discoveryService{
		ctx, nil, userRepo, nodeRepo,
		config.Config().Version, "", handlers,
		retrier.New(time.Second, 5, retrier.ArithmeticalBackoff),
		newRateLimiter(16, 1),
		newDiscoveryCache(),
		addrInfos,
		make(chan warpnet.PeerAddrInfo, 1000), make(chan struct{}),
		new(atomic.Bool),
	}
}

func NewBootstrapDiscoveryService(ctx context.Context, handlers ...DiscoveryHandler) *discoveryService {
	addrs := make(map[warpnet.WarpPeerID][]warpnet.WarpAddress)
	addrInfos, _ := config.Config().Node.AddrInfos()
	for _, info := range addrInfos {
		addrs[info.ID] = info.Addrs
	}
	return NewDiscoveryService(ctx, nil, nil, handlers...)
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
	s.pskHash = n.NodeInfo().PSKHash

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				log.Errorf("discovery: context closed")
				return
			case <-s.stopChan:
				return
			case info, ok := <-s.discoveryChan:
				if !ok {
					log.Infoln("discovery: service closed")
					return
				}
				s.handle(info)
			}
		}
	}()
	return s.syncBootstrapDiscovery()
}

func (s *discoveryService) syncBootstrapDiscovery() error {
	defer func() {
		s.syncDone.Store(true)
	}()

	for _, info := range s.bootstrapAddrs {
		if s.node.NodeInfo().ID == info.ID {
			continue
		}
		s.discoveryChan <- info
	}

	if s.node.NodeInfo().IsBootstrap() {
		return nil
	}

	tryouts := 30
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		var isAllDiscovered = true

		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-s.stopChan:
			return nil
		case <-ticker.C:
			for _, info := range s.bootstrapAddrs {
				if !s.cache.IsChallengedAlready(info.ID) {
					isAllDiscovered = false
					break
				}
			}

			if isAllDiscovered {
				log.Infof("discovery: all bootstrap addresses discovered")
				return nil
			}

			tryouts--
			if tryouts == 0 {
				return warpnet.WarpError("discovery: all discovery attempts failed")
			}
		}
	}
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
	close(s.stopChan)
	close(s.discoveryChan)
}

func (s *discoveryService) DefaultDiscoveryHandler(peerInfo warpnet.PeerAddrInfo) {
	if s == nil {
		return
	}
	defer func() { recover() }()

	if peerInfo.ID == s.node.NodeInfo().ID {
		return
	}

	ok, err := s.nodeRepo.IsBlocklisted(s.ctx, peerInfo.ID)
	if ok && err == nil {
		log.Infof("discovery: found blocklisted peer: %s", peerInfo.ID.String())
		return
	}

	err = s.node.Connect(peerInfo)
	if errors.Is(err, backoff.ErrBackoffEnabled) {
		return
	}
	if err != nil {
		if errors.Is(err, warpnet.ErrAllDialsFailed) {
			err = warpnet.ErrAllDialsFailed
		}
		log.Errorf("discovery: default handler: connecting to peer %s: %v\n", peerInfo.ID.String(), err)
		return
	}

	err = s.requestChallenge(peerInfo)
	if errors.Is(err, ErrChallengeMismatch) || errors.Is(err, ErrChallengeSignatureInvalid) {
		log.Warnf("discovery: default handler: challenge is invalid for peer: %s\n", peerInfo.ID.String())
		_ = s.nodeRepo.Blocklist24h(context.Background(), peerInfo.ID)
		return
	}
	if err != nil {
		log.Errorf(
			"discovery: default handler: failed to request challenge for peer %s: %v\n",
			peerInfo.ID, err,
		)
		return
	}
	s.node.Peerstore().AddAddrs(peerInfo.ID, peerInfo.Addrs, time.Hour*8)

	for _, h := range s.handlers {
		h(peerInfo)
	}
	return
}

const dropMessagesLimit = 5

func (s *discoveryService) HandlePeerFound(pi warpnet.PeerAddrInfo) {
	if s == nil {
		return
	}
	defer func() { recover() }()

	if !s.limiter.Allow() {
		log.Debugf("discovery: limited by rate limiter: %s", pi.ID.String())
		return
	}

	if len(s.discoveryChan) == cap(s.discoveryChan) {
		log.Warnf("discovery: channel overflow %d", cap(s.discoveryChan))
		for i := 0; i < dropMessagesLimit; i++ {
			<-s.discoveryChan // drop old data
		}

	}
	s.discoveryChan <- pi
}

func (s *discoveryService) handle(pi warpnet.PeerAddrInfo) {
	if s == nil || s.node == nil || s.nodeRepo == nil || s.userRepo == nil {
		return
	}

	if pi.ID == "" || len(pi.Addrs) == 0 {
		return
	}

	if pi.ID == s.node.NodeInfo().ID {
		return
	}

	ok, err := s.nodeRepo.IsBlocklisted(s.ctx, pi.ID)
	if err != nil {
		log.Errorf("discovery: failed to check blocklist: %s", err)
	}
	if ok {
		log.Infof("discovery: found blocklisted peer: %s", pi.ID.String())
		return
	}

	for _, h := range s.handlers {
		h(pi)
	}

	if !hasPublicAddresses(pi.Addrs) {
		log.Debugf("discovery: peer %s has no public addresses: %v", pi.ID.String(), pi.Addrs)
		return
	}

	err = s.node.Connect(pi)
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
		_ = s.nodeRepo.Blocklist24h(context.Background(), pi.ID)
		return
	}
	if err != nil {
		log.Errorf("discovery: failed to request challenge for peer %s: %v\n", pi.ID, err)
		return
	}

	info, err := s.requestNodeInfo(pi)
	if err != nil {
		log.Errorf("discovery: %v", err)
		return
	}
	if info.PSKHash != s.pskHash {
		log.Errorf("discovery: PSK hashes mismatch %s", info.PSKHash)
		return
	}

	if info.IsBootstrap() {
		return
	}

	existedUser, err := s.userRepo.GetByNodeID(pi.ID.String())
	if !errors.Is(err, database.ErrUserNotFound) && !existedUser.IsOffline {
		return
	}

	fmt.Printf("\033[1mdiscovery: connected to new peer: %s \033[0m\n", pi.String())

	user, err := s.requestNodeUser(pi, info.OwnerId)
	if err != nil {
		log.Errorf("discovery: %v", err)
		return
	}

	newUser, err := s.userRepo.Create(user)
	if errors.Is(err, database.ErrUserAlreadyExists) {
		newUser, _ = s.userRepo.Update(user.Id, user)
		return
	}
	if err != nil {
		log.Errorf("discovery: failed to create user from new peer: %s", err)
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

const (
	ErrChallengeMismatch         warpnet.WarpError = "challenge mismatch"
	ErrChallengeSignatureInvalid warpnet.WarpError = "invalid challenge signature"
)

func (s *discoveryService) requestChallenge(pi warpnet.PeerAddrInfo) error {
	if s == nil {
		return errors.New("nil discovery service")
	}
	if s.cache.IsChallengedAlready(pi.ID) {
		log.Infof("discovery: peer %s already challenged", pi.ID.String())
		return nil
	}

	nonce := rand.Int64()
	ownChallenge, location, err := security.GenerateChallenge(root.GetCodeBase(), nonce)
	if err != nil {
		return err
	}

	resp, err := s.node.GenericStream(
		pi.ID.String(),
		event.PUBLIC_GET_NODE_CHALLENGE,
		event.GetChallengeEvent{
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

	var challengeResp event.GetChallengeResponse
	err = json.JSON.Unmarshal(resp, &challengeResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal challenge from new peer: %s %v", resp, err)
	}

	challengeRespOrigin, err := hex.DecodeString(challengeResp.Challenge)
	if err != nil {
		return fmt.Errorf("failed to decode challenge origin: %v", err)
	}

	if !bytes.Equal(ownChallenge, challengeRespOrigin) {
		return ErrChallengeMismatch
	}

	peerstorePubKey := s.node.Peerstore().PubKey(pi.ID)
	if peerstorePubKey == nil {
		return fmt.Errorf("peer %s has no public key", pi.ID.String())
	}
	if peerstorePubKey.Type() != pb.KeyType_Ed25519 {
		return errors.New("peer is not an Ed25519 public key")
	}

	rawPubKey, err := peerstorePubKey.Raw()
	if err != nil {
		return err
	}

	decodedSig, err := base64.StdEncoding.DecodeString(challengeResp.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature base64: %v", err)
	}

	if !ed25519.Verify(rawPubKey, challengeRespOrigin, decodedSig) {
		return ErrChallengeSignatureInvalid
	}

	s.cache.SetAsChallenged(pi.ID)
	return nil
}

func (s *discoveryService) requestNodeInfo(pi warpnet.PeerAddrInfo) (info warpnet.NodeInfo, err error) {
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

	err = json.JSON.Unmarshal(infoResp, &info)
	if err != nil {
		return info, fmt.Errorf("failed to unmarshal info from new peer: %s %v", infoResp, err)
	}
	if info.OwnerId == "" {
		return info, fmt.Errorf("node info %s has no owner", pi.ID.String())
	}
	return info, nil
}

func (s *discoveryService) requestNodeUser(pi warpnet.PeerAddrInfo, userId string) (user domain.User, err error) {
	if s == nil {
		return user, errors.New("nil discovery service")
	}

	getUserEvent := event.GetUserEvent{UserId: userId}

	userResp, err := s.node.GenericStream(pi.ID.String(), event.PUBLIC_GET_USER, getUserEvent)
	if err != nil {
		return user, fmt.Errorf("failed to user data from new peer %s: %v", pi.ID.String(), err)
	}

	if userResp == nil || len(userResp) == 0 {
		return user, fmt.Errorf("no user response from new peer %s", pi.String())
	}

	err = json.JSON.Unmarshal(userResp, &user)
	if err != nil {
		return user, fmt.Errorf("failed to unmarshal user from new peer: %v", err)
	}

	user.IsOffline = false
	user.NodeId = pi.ID.String()
	user.Latency = int64(s.node.Peerstore().LatencyEWMA(pi.ID))
	return user, nil
}

func hasPublicAddresses(addrs []warpnet.WarpAddress) bool {
	for _, addr := range addrs {
		if warpnet.IsPublicMultiAddress(addr) {
			return true
		}
	}
	return false
}
