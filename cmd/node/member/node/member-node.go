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

package node

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	log "github.com/sirupsen/logrus"
	"time"
)

type MemberNode struct {
	ctx context.Context

	node *node.WarpNode
	opts []warpnet.WarpOption

	discService          DiscoveryHandler
	mdnsService          MDNSStarterCloser
	pubsubService        PubSubProvider
	consensusService     ConsensusServicer
	dHashTable           DistributedHashTableCloser
	nodeRepo             NodeProvider
	retrier              retrier.Retrier
	authRepo             AuthProvider
	userRepo             UserProvider
	followRepo           FollowStorer
	db                   Storer
	privKey              ed25519.PrivateKey
	ownerId, selfHashHex string
	pseudoNode           PseudoStreamer
}

func NewMemberNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	selfHashHex string,
	version *semver.Version,
	authRepo AuthProvider,
	db Storer,
) (_ *MemberNode, err error) {
	if len(privKey) == 0 {
		return nil, errors.New("private key is required")
	}
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

	followeeIds, err := fetchFolloweeIds(owner.UserId, followRepo)
	if err != nil {
		return nil, err
	}
	pubsubHandlers := []pubsub.TopicHandler{
		pubsub.NewDiscoveryTopicHandler(discService.WrapPubSubDiscovery(discService.HandlePeerFound)),
		pubsub.NewTransitModerationHandler(),
	}
	pubsubHandlers = append(pubsubHandlers, pubsub.PrefollowUsers(followeeIds...)...)
	pubsubService := pubsub.NewPubSub(
		ctx,
		pubsubHandlers...,
	)

	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		return nil, err
	}

	dHashTable := dht.NewDHTable(
		ctx,
		dht.RoutingStore(nodeRepo),
		dht.EnableRendezvous(),
		dht.AddPeerCallbacks(discService.HandlePeerFound),
		dht.BootstrapNodes(infos...),
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

	currentNodeID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}

	opts := []warpnet.WarpOption{
		node.WarpIdentity(privKey),
		libp2p.Peerstore(store),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		),
		libp2p.Routing(dHashTable.StartRouting),
		node.EnableAutoRelayWithStaticRelays(infos, currentNodeID)(),
	}

	opts = append(opts, node.CommonOptions...)

	mn := &MemberNode{
		ctx:           ctx,
		opts:          opts,
		discService:   discService,
		mdnsService:   mdnsService,
		pubsubService: pubsubService,
		dHashTable:    dHashTable,
		nodeRepo:      nodeRepo,
		retrier:       retrier.New(time.Second, 5, retrier.FixedBackoff),
		userRepo:      userRepo,
		followRepo:    followRepo,
		authRepo:      authRepo,
		db:            db,
		privKey:       privKey,
		ownerId:       owner.UserId,
		selfHashHex:   selfHashHex,
		pseudoNode:    mastodonPseudoNode,
	}

	//mn.consensusService = consensus.NewGossipConsensus(
	//	ctx, pubsubService,
	//)

	return mn, nil
}

func (m *MemberNode) Start() (err error) {
	m.node, err = node.NewWarpNode(
		m.ctx,
		m.opts...,
	)
	if err != nil {
		return fmt.Errorf("member: failed to init node: %v", err)
	}
	m.setupHandlers(m.authRepo, m.userRepo, m.followRepo, m.db, m.privKey)
	if m.pseudoNode != nil {
		m.node.Node().Peerstore().AddAddrs(m.pseudoNode.ID(), m.pseudoNode.Addrs(), time.Hour*24)
	}

	m.pubsubService.Run(m)

	if err := m.discService.Run(m); err != nil {
		return err
	}

	m.mdnsService.Start(m)

	nodeInfo := m.NodeInfo()

	ownerUser, err := m.userRepo.Get(nodeInfo.OwnerId)
	if err != nil {
		return err
	}

	//if err := m.consensusService.Start(m); err != nil {
	//	return err
	//}

	ev := event.ValidationEvent{
		ValidatedNodeID: nodeInfo.ID.String(),
		User:            &ownerUser,
	}
	go m.consensusService.AskValidation(ev)

	println()
	fmt.Printf(
		"\033[1mNODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func fetchFolloweeIds(ownerId string, followRepo FollowStorer) (ids []string, err error) {
	if followRepo == nil {
		return ids, nil
	}

	var (
		nextCursor string
		limit      = uint64(20)
	)
	for {
		followees, cur, err := followRepo.GetFollowees(ownerId, &limit, &nextCursor)
		if err != nil {
			return ids, err
		}
		for _, f := range followees {
			if f.Followee == ownerId {
				continue
			}
			ids = append(ids, f.Followee)
		}
		if len(followees) < int(limit) {
			break
		}
		nextCursor = cur
	}
	return ids, nil
}

func (m *MemberNode) Connect(p warpnet.WarpAddrInfo) error {
	if m == nil || m.node == nil {
		return nil
	}
	if m.pseudoNode != nil && m.pseudoNode.IsMastodonID(p.ID) {
		return nil
	}

	return m.node.Connect(p)
}

func (m *MemberNode) NodeInfo() warpnet.NodeInfo {
	bi := m.node.BaseNodeInfo()
	bi.OwnerId = m.ownerId
	return bi
}

func (m *MemberNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if m == nil || m.node == nil {
		return nil, nil
	}
	return m.node.SelfStream(path, data)
}

type streamNodeID = string

func (m *MemberNode) GenericStream(nodeIdStr streamNodeID, path stream.WarpRoute, data any) (_ []byte, err error) {
	if m == nil {
		return nil, nil
	}
	nodeId := warpnet.FromStringToPeerID(nodeIdStr)

	var isMastodonID bool
	if m.pseudoNode != nil {
		isMastodonID = m.pseudoNode.IsMastodonID(nodeId)
	}

	peerInfo := m.node.Node().Peerstore().PeerInfo(nodeId)
	if len(peerInfo.Addrs) == 0 && !isMastodonID {
		log.Warningf("node %v is offline", nodeId)
		return nil, warpnet.ErrNodeIsOffline
	}

	if isMastodonID {
		log.Debugf("stream: peer %s is mastodon", nodeIdStr)
		return m.pseudoNode.Route(path, data)
	}

	bt, err := m.node.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		m.setUserOffline(nodeIdStr)
		return bt, err
	}

	if err != nil {
		ctx, cancelF := context.WithTimeout(context.Background(), time.Second*10)
		defer cancelF()
		_ = m.retrier.Try(ctx, func() error {
			bt, err = m.node.Stream(nodeId, path, data) // TODO dead letters queue
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

	m.node.SetStreamHandlers(
		[]warpnet.WarpStreamHandler{
			{
				event.PRIVATE_POST_PAIR,
				handler.StreamNodesPairingHandler(authNodeInfo),
			},
			{
				event.INTERNAL_POST_NODE_VALIDATE,
				handler.StreamValidateHandler(m.consensusService),
			},
			{
				event.PUBLIC_POST_NODE_VALIDATION_RESULT,
				handler.StreamValidationResponseHandler(m.consensusService),
			},
			{
				event.PUBLIC_POST_NODE_CHALLENGE,
				handler.StreamChallengeHandler(root.GetCodeBase(), privKey),
			},
			{
				event.PUBLIC_GET_INFO,
				handler.StreamGetInfoHandler(m, m.discService.HandlePeerFound),
			},
			{
				event.PRIVATE_GET_STATS,
				handler.StreamGetStatsHandler(m, db),
			},
			{
				event.PRIVATE_GET_TIMELINE,
				handler.StreamTimelineHandler(timelineRepo),
			},
			{
				event.PRIVATE_POST_TWEET,
				handler.StreamNewTweetHandler(m.pubsubService, authRepo, tweetRepo, timelineRepo),
			},
			{
				event.PRIVATE_DELETE_TWEET,
				handler.StreamDeleteTweetHandler(m.pubsubService, authRepo, tweetRepo, likeRepo),
			},
			{
				event.PUBLIC_POST_REPLY,
				handler.StreamNewReplyHandler(replyRepo, userRepo, m),
			},
			{
				event.PUBLIC_DELETE_REPLY,
				handler.StreamDeleteReplyHandler(tweetRepo, userRepo, replyRepo, m),
			},
			{
				event.PUBLIC_POST_FOLLOW,
				handler.StreamFollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m),
			},
			{
				event.PUBLIC_POST_UNFOLLOW,
				handler.StreamUnfollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m),
			},
			{
				event.PUBLIC_GET_USER,
				handler.StreamGetUserHandler(tweetRepo, followRepo, userRepo, authRepo, m),
			},
			{
				event.PUBLIC_GET_USERS,
				handler.StreamGetUsersHandler(userRepo, m),
			},
			{
				event.PUBLIC_GET_WHOTOFOLLOW,
				handler.StreamGetWhoToFollowHandler(authRepo, userRepo, followRepo),
			},
			{
				event.PUBLIC_GET_TWEETS,
				handler.StreamGetTweetsHandler(tweetRepo, userRepo, m),
			},
			{
				event.PUBLIC_GET_TWEET,
				handler.StreamGetTweetHandler(tweetRepo),
			},
			{
				event.PUBLIC_GET_TWEET_STATS,
				handler.StreamGetTweetStatsHandler(likeRepo, tweetRepo, replyRepo, userRepo, m),
			},
			{
				event.PUBLIC_GET_REPLY,
				handler.StreamGetReplyHandler(replyRepo),
			},
			{
				event.PUBLIC_GET_REPLIES,
				handler.StreamGetRepliesHandler(replyRepo, userRepo, m),
			},
			{
				event.PUBLIC_GET_FOLLOWERS,
				handler.StreamGetFollowersHandler(authRepo, userRepo, followRepo, m),
			},
			{
				event.PUBLIC_GET_FOLLOWEES,
				handler.StreamGetFolloweesHandler(authRepo, userRepo, followRepo, m),
			},
			{
				event.PUBLIC_POST_LIKE,
				handler.StreamLikeHandler(likeRepo, userRepo, m),
			},
			{
				event.PUBLIC_POST_UNLIKE,
				handler.StreamUnlikeHandler(likeRepo, userRepo, m),
			},
			{
				event.PRIVATE_POST_USER,
				handler.StreamUpdateProfileHandler(authRepo, userRepo),
			},
			{
				event.PUBLIC_POST_RETWEET,
				handler.StreamNewReTweetHandler(userRepo, tweetRepo, timelineRepo, m),
			},
			{
				event.PUBLIC_POST_UNRETWEET,
				handler.StreamUnretweetHandler(tweetRepo, userRepo, m),
			},
			{
				event.PUBLIC_POST_CHAT,
				handler.StreamCreateChatHandler(chatRepo, userRepo, m),
			},
			{
				event.PRIVATE_DELETE_CHAT,
				handler.StreamDeleteChatHandler(chatRepo, authRepo),
			},
			{
				event.PRIVATE_GET_CHATS,
				handler.StreamGetUserChatsHandler(chatRepo, authRepo),
			},
			{
				event.PUBLIC_POST_MESSAGE,
				handler.StreamSendMessageHandler(chatRepo, userRepo, m),
			},
			{
				event.PRIVATE_DELETE_MESSAGE,
				handler.StreamDeleteMessageHandler(chatRepo, authRepo),
			},
			{
				event.PRIVATE_GET_MESSAGE,
				handler.StreamGetMessageHandler(chatRepo, authRepo),
			},
			{
				event.PRIVATE_GET_MESSAGES,
				handler.StreamGetMessagesHandler(chatRepo, authRepo),
			},
			{
				event.PRIVATE_GET_CHAT,
				handler.StreamGetUserChatHandler(chatRepo, authRepo),
			},
			{
				event.PRIVATE_POST_UPLOAD_IMAGE,
				handler.StreamUploadImageHandler(m, mediaRepo, userRepo),
			},
			{
				event.PUBLIC_GET_IMAGE,
				handler.StreamGetImageHandler(m, mediaRepo, userRepo),
			},
			{
				event.PUBLIC_POST_MODERATION_RESULT, // TODO protect this endpoint
				handler.StreamModerationResultHandler(m.pubsubService, userRepo, tweetRepo, timelineRepo),
			},
		}...,
	)
}

func (m *MemberNode) Node() warpnet.P2PNode {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node()
}

func (m *MemberNode) Peerstore() warpnet.WarpPeerstore {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node().Peerstore()
}

func (m *MemberNode) Network() warpnet.WarpNetwork {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node().Network()
}

func (m *MemberNode) SimpleConnect(info warpnet.WarpAddrInfo) error {
	return m.node.Node().Connect(m.ctx, info)
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
	m.node.StopNode()
}
