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

package member

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/consensus/gossip"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node/base"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)

type MemberNode struct {
	*base.WarpNode

	ctx                  context.Context
	discService          DiscoveryHandler
	mdnsService          MDNSStarterCloser
	pubsubService        PubSubProvider
	consensusService     ConsensusServicer
	dHashTable           DistributedHashTableCloser
	nodeRepo             NodeProvider
	retrier              retrier.Retrier
	userRepo             UserFetcher
	ownerId, selfHashHex string
}

func NewMemberNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
	version *semver.Version,
	authRepo AuthProvider,
	db Storer,
	interruptChan chan os.Signal,
) (_ *MemberNode, err error) {
	nodeRepo, err := database.NewNodeRepo(db, version)
	if err != nil {
		return nil, err
	}
	if err := nodeRepo.AddSelfHash(selfHashHex, version.String()); err != nil {
		return nil, err
	}

	store, err := warpnet.NewPeerstore(ctx, nodeRepo)
	if err != nil {
		return nil, err
	}

	userRepo := database.NewUserRepo(db)
	followRepo := database.NewFollowRepo(db)
	owner := authRepo.GetOwner()

	discService := discovery.NewDiscoveryService(ctx, userRepo, nodeRepo)
	mdnsService := mdns.NewMulticastDNS(ctx, discService.HandlePeerFound)
	pubsubService := pubsub.NewPubSub(
		ctx, followRepo, owner.UserId, discService.HandlePeerFound,
	)

	dHashTable := dht.NewDHTable(
		ctx, nodeRepo,
		nil, discService.HandlePeerFound,
	)

	mastodonPseudoNode, err := mastodon.NewWarpnetMastodonPseudoNode(ctx, version)
	if err != nil {
		log.Errorf("mastodon: creating mastodon pseudo-node: %v", err)
	}
	if mastodonPseudoNode != nil {
		_, _ = userRepo.Create(mastodonPseudoNode.WarpnetUser())
		_, _ = userRepo.Update(mastodonPseudoNode.WarpnetUser().Id, mastodonPseudoNode.WarpnetUser())
		_, _ = userRepo.Create(mastodonPseudoNode.DefaultUser())
		_, _ = userRepo.Update(mastodonPseudoNode.DefaultUser().Id, mastodonPseudoNode.DefaultUser())
	}

	node, err := base.NewWarpNode(
		ctx,
		privKey,
		store,
		psk,
		mastodonPseudoNode,
		[]string{
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		},
		dHashTable.StartRouting,
	)
	if err != nil {
		return nil, fmt.Errorf("member: failed to init node: %v", err)
	}

	if mastodonPseudoNode != nil {
		node.Peerstore().AddAddrs(mastodonPseudoNode.ID(), mastodonPseudoNode.Addrs(), time.Hour*24)
	}

	mn := &MemberNode{
		WarpNode:      node,
		ctx:           ctx,
		discService:   discService,
		mdnsService:   mdnsService,
		pubsubService: pubsubService,
		dHashTable:    dHashTable,
		nodeRepo:      nodeRepo,
		retrier:       retrier.New(time.Second, 5, retrier.FixedBackoff),
		userRepo:      userRepo,
		ownerId:       owner.UserId,
		selfHashHex:   selfHashHex,
	}

	mn.consensusService = gossip.NewGossipConsensus(
		ctx, pubsubService, mn, interruptChan, nodeRepo.ValidateSelfHash, userRepo.ValidateUserID,
	)

	mn.setupHandlers(authRepo, userRepo, followRepo, db, privKey)
	return mn, nil
}

func (m *MemberNode) Start(clientNode ClientNodeStreamer) error {
	m.pubsubService.Run(m, clientNode)
	if err := m.discService.Run(m); err != nil {
		return err
	}
	m.mdnsService.Start(m)

	nodeInfo := m.NodeInfo()

	ownerUser, _ := m.userRepo.Get(nodeInfo.OwnerId)

	if err := m.consensusService.Start(event.ValidationEvent{
		ValidatedNodeID: nodeInfo.ID.String(),
		SelfHashHex:     m.selfHashHex,
		User:            &ownerUser,
	}); err != nil {
		return err
	}

	println()
	fmt.Printf(
		"\033[1mNODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func (m *MemberNode) NodeInfo() warpnet.NodeInfo {
	bi := m.BaseNodeInfo()
	bi.OwnerId = m.ownerId
	return bi
}

type streamNodeID = string

func (m *MemberNode) GenericStream(nodeIdStr streamNodeID, path stream.WarpRoute, data any) (_ []byte, err error) {
	if m == nil {
		return nil, nil
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)

	bt, err := m.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		m.setUserOffline(nodeIdStr)
		return bt, err
	}

	if err != nil {
		ctx, cancelF := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelF()
		m.retrier.Try(ctx, func() error {
			bt, err = m.Stream(nodeId, path, data) // TODO dead letters queue
			return err
		})
	}
	return bt, err
}

func (m *MemberNode) setUserOffline(nodeIdStr streamNodeID) {
	if m == nil {
		return
	}
	u, err := m.userRepo.GetByNodeID(nodeIdStr)
	if errors.Is(err, database.ErrUserNotFound) {
		return
	}
	if err != nil {
		log.Warningf("member: stream: failed to get user: %v", err)
		return
	}
	u.IsOffline = true
	_, err = m.userRepo.Update(u.Id, u)
	if err != nil {
		log.Warningf("member: stream: failed to set user offline: %v", err)
		return
	}
}

func (m *MemberNode) setupHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
	db Storer,
	privKey ed25519.PrivateKey,
) {
	if m == nil {
		panic("member: setup handlers: nil node")
	}
	timelineRepo := database.NewTimelineRepo(db)
	tweetRepo := database.NewTweetRepo(db)
	replyRepo := database.NewRepliesRepo(db)
	likeRepo := database.NewLikeRepo(db)
	chatRepo := database.NewChatRepo(db)
	mediaRepo := database.NewMediaRepo(db)

	authNodeInfo := domain.AuthNodeInfo{
		Identity: domain.Identity{Owner: authRepo.GetOwner(), Token: authRepo.SessionToken()},
		NodeInfo: m.NodeInfo(),
	}

	mw := middleware.NewWarpMiddleware()
	logMw := mw.LoggingMiddleware
	authMw := mw.AuthMiddleware
	unwrapMw := mw.UnwrapStreamMiddleware
	m.SetStreamHandler(
		event.PRIVATE_POST_NODE_VALIDATE,
		logMw(unwrapMw(handler.StreamValidateHandler(m.consensusService))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_NODE_VALIDATION_RESULT,
		logMw(unwrapMw(handler.StreamValidationResponseHandler(m.consensusService))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_NODE_CHALLENGE,
		logMw(unwrapMw(handler.StreamChallengeHandler(root.GetCodeBase(), privKey))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_INFO,
		logMw(handler.StreamGetInfoHandler(m, m.discService.HandlePeerFound)),
	)

	m.SetStreamHandler(
		event.PRIVATE_GET_STATS,
		logMw(authMw(unwrapMw(handler.StreamGetStatsHandler(m, db)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_POST_PAIR,
		logMw(authMw(unwrapMw(handler.StreamNodesPairingHandler(authNodeInfo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_GET_TIMELINE,
		logMw(authMw(unwrapMw(handler.StreamTimelineHandler(timelineRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_POST_TWEET,
		logMw(authMw(unwrapMw(handler.StreamNewTweetHandler(m.pubsubService, authRepo, tweetRepo, timelineRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_DELETE_TWEET,
		logMw(authMw(unwrapMw(handler.StreamDeleteTweetHandler(m.pubsubService, authRepo, tweetRepo, likeRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_REPLY,
		logMw(authMw(unwrapMw(handler.StreamNewReplyHandler(replyRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_DELETE_REPLY,
		logMw(authMw(unwrapMw(handler.StreamDeleteReplyHandler(tweetRepo, userRepo, replyRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_FOLLOW,
		logMw(authMw(unwrapMw(handler.StreamFollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_UNFOLLOW,
		logMw(authMw(unwrapMw(handler.StreamUnfollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_USER,
		logMw(authMw(unwrapMw(handler.StreamGetUserHandler(tweetRepo, followRepo, userRepo, authRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_USERS,
		logMw(authMw(unwrapMw(handler.StreamGetUsersHandler(userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_WHOTOFOLLOW,
		logMw(authMw(unwrapMw(handler.StreamGetWhoToFollowHandler(authRepo, userRepo, followRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_TWEETS,
		logMw(authMw(unwrapMw(handler.StreamGetTweetsHandler(tweetRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_TWEET,
		logMw(authMw(unwrapMw(handler.StreamGetTweetHandler(tweetRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_TWEET_STATS,
		logMw(authMw(unwrapMw(handler.StreamGetTweetStatsHandler(likeRepo, tweetRepo, replyRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_REPLY,
		logMw(authMw(unwrapMw(handler.StreamGetReplyHandler(replyRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_REPLIES,
		logMw(authMw(unwrapMw(handler.StreamGetRepliesHandler(replyRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_FOLLOWERS,
		logMw(authMw(unwrapMw(handler.StreamGetFollowersHandler(authRepo, userRepo, followRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_FOLLOWEES,
		logMw(authMw(unwrapMw(handler.StreamGetFolloweesHandler(authRepo, userRepo, followRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_LIKE,
		logMw(authMw(unwrapMw(handler.StreamLikeHandler(likeRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_UNLIKE,
		logMw(authMw(unwrapMw(handler.StreamUnlikeHandler(likeRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_POST_USER,
		logMw(authMw(unwrapMw(handler.StreamUpdateProfileHandler(authRepo, userRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_RETWEET,
		logMw(authMw(unwrapMw(handler.StreamNewReTweetHandler(userRepo, tweetRepo, timelineRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_UNRETWEET,
		logMw(authMw(unwrapMw(handler.StreamUnretweetHandler(tweetRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_CHAT,
		logMw(authMw(unwrapMw(handler.StreamCreateChatHandler(chatRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_DELETE_CHAT,
		logMw(authMw(unwrapMw(handler.StreamDeleteChatHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_GET_CHATS,
		logMw(authMw(unwrapMw(handler.StreamGetUserChatsHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_POST_MESSAGE,
		logMw(authMw(unwrapMw(handler.StreamSendMessageHandler(chatRepo, userRepo, m)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_DELETE_MESSAGE,
		logMw(authMw(unwrapMw(handler.StreamDeleteMessageHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_GET_MESSAGE,
		logMw(authMw(unwrapMw(handler.StreamGetMessageHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_GET_MESSAGES,
		logMw(authMw(unwrapMw(handler.StreamGetMessagesHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_GET_CHAT,
		logMw(authMw(unwrapMw(handler.StreamGetUserChatHandler(chatRepo, authRepo)))),
	)
	m.SetStreamHandler(
		event.PRIVATE_POST_UPLOAD_IMAGE,
		logMw(authMw(unwrapMw(handler.StreamUploadImageHandler(m, mediaRepo, userRepo)))),
	)
	m.SetStreamHandler(
		event.PUBLIC_GET_IMAGE,
		logMw(authMw(unwrapMw(handler.StreamGetImageHandler(m, mediaRepo, userRepo)))),
	)
}

func (m *MemberNode) Stop() {
	if m == nil {
		return
	}
	if m.discService != nil {
		m.discService.Close()
	}
	if m.mdnsService != nil {
		m.mdnsService.Close()
	}
	if m.pubsubService != nil {
		if err := m.pubsubService.Close(); err != nil {
			log.Errorf("member: failed to close pubsub: %v", err)
		}
	}
	if m.dHashTable != nil {
		m.dHashTable.Close()
	}
	if m.consensusService != nil {
		m.consensusService.Close()
	}
	if m.nodeRepo != nil {
		if err := m.nodeRepo.Close(); err != nil {
			log.Errorf("member: failed to close node repo: %v", err)
		}
	}
	m.StopNode()
}
