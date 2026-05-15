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
	"github.com/Warp-net/warpnet/core/challenge"
	"time"

	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	memberPubSub "github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	log "github.com/sirupsen/logrus"
)

type MetricsOnlinePusher interface {
	PushStatusOnline(nodeId string)
	PushStatusOffline(nodeId string)
}

type MemberNode struct {
	ctx context.Context

	node *node.WarpNode
	opts []warpnet.WarpOption

	discService                   DiscoveryHandler
	mdnsService                   MDNSStarterCloser
	pubsubService                 PubSubProvider
	dHashTable                    DistributedHashTableCloser
	nodeRepo                      NodeProvider
	statsRepo                     StatsProvider
	authRepo                      AuthProvider
	userRepo                      UserProvider
	deviceRepo                    DeviceProvider
	followRepo                    FollowStorer
	db                            Storer
	statsDb                       StatsStorer
	privKey                       ed25519.PrivateKey
	ownerId, selfHashHex, network string
	pseudoNode                    PseudoStreamer
	retrier                       retrier.Retrier
}

func NewMemberNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
	selfHashHex string,
	version *semver.Version,
	authRepo AuthProvider,
	db Storer,
	bootstrapNodes []warpnet.WarpAddrInfo,
	metrics MetricsOnlinePusher,
) (_ *MemberNode, err error) {
	if len(privKey) == 0 {
		return nil, node.ErrPrivateKeyRequired
	}
	nodeRepo := database.NewNodeRepo(db)
	store, err := warpnet.NewPeerstore(ctx, nodeRepo)
	if err != nil {
		return nil, err
	}

	statsRepo := database.NewStatsRepo(db)
	userRepo := database.NewUserRepo(db)
	followRepo := database.NewFollowRepo(db)
	deviceRepo := database.NewDevicesRepo(db)
	owner := authRepo.GetOwner()

	challenger := challenge.NewSpoofChallenger(ctx)

	discService := discovery.NewDiscoveryService(ctx, userRepo, nodeRepo, challenger, metrics)
	mdnsService := mdns.NewMulticastDNS(ctx, discService.DiscoveryHandlerMDNS)

	followingIds, err := fetchFollowingIds(owner.UserId, followRepo)
	if err != nil {
		return nil, err
	}

	pubSubHandlers := memberPubSub.PrefollowHandlers(followingIds...)
	pubSubHandlers = append(
		pubSubHandlers,
		memberPubSub.NewBootstrapDiscoveryTopicHandler(discService.DiscoveryHandlerPubSub),
	)
	pubsubService := memberPubSub.NewPubSub(ctx, pubSubHandlers...)

	warpNetwork := config.Config().Node.Network

	dHashTable := dht.NewDHTable(
		ctx,
		dht.RoutingStore(nodeRepo),
		dht.AddPeerCallbacks(discService.DiscoveryHandlerDHT),
		dht.BootstrapNodes(bootstrapNodes...),
		dht.Network(warpNetwork),
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

	opts := []warpnet.WarpOption{ //nolint:prealloc
		node.WarpIdentity(privKey),
		libp2p.Peerstore(store),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip6/%s/tcp/%s", config.Config().Node.HostV6, config.Config().Node.Port),
			fmt.Sprintf("/ip4/%s/tcp/%s", config.Config().Node.HostV4, config.Config().Node.Port),
		),
		libp2p.Routing(dHashTable.StartRouting),
		node.EnableAutoRelayWithStaticRelays(bootstrapNodes, ownNodeId)(),
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
		statsRepo:     statsRepo,
		retrier:       retrier.New(time.Second, 5, retrier.FixedBackoff),
		userRepo:      userRepo,
		followRepo:    followRepo,
		deviceRepo:    deviceRepo,
		authRepo:      authRepo,
		db:            db,
		privKey:       privKey,
		ownerId:       owner.UserId,
		selfHashHex:   selfHashHex,
		pseudoNode:    mastodonPseudoNode,
		network:       warpNetwork,
	}

	return mn, nil
}

func (m *MemberNode) Start() (err error) {
	m.node, err = node.NewWarpNode(
		m.ctx,
		m.opts...,
	)
	if err != nil {
		return fmt.Errorf("member: failed to start node: %w", err)
	}

	m.pubsubService.Run(m)
	if err := m.discService.Run(m); err != nil {
		return err
	}

	m.mdnsService.Start(m)

	nodeInfo := m.NodeInfo()

	crdtBroadcaster, err := crdt.NewGossipBroadcaster(m.ctx, m.pubsubService.Gossip())
	if err != nil {
		return fmt.Errorf("member: failed to start crdt gossip broadcaster: %w", err)
	}
	m.statsDb, err = crdt.NewCRDTStatsStore(
		m.ctx, crdtBroadcaster, m.statsRepo, m.node.Node(), m.dHashTable,
	)
	if err != nil {
		return fmt.Errorf("member: failed to initialize stats store: %w", err)
	}

	m.setupHandlers(m.authRepo, m.userRepo, m.followRepo, m.db, m.statsDb, m.privKey)

	for _, addr := range m.dHashTable.BootstrapNodes() {
		m.SetMaxNodePriority(addr.ID)
	}

	println()
	fmt.Printf(
		"\033[1mNODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func fetchFollowingIds(ownerId string, followRepo FollowStorer) (ids []string, err error) {
	if followRepo == nil {
		return ids, nil
	}

	var (
		nextCursor string
		limit      = uint64(20)
	)
	for {
		followings, cur, err := followRepo.GetFollowings(ownerId, &limit, &nextCursor)
		if err != nil {
			return ids, err
		}
		for _, id := range followings {
			if id == ownerId {
				continue
			}
			ids = append(ids, id)
		}
		if uint64(len(followings)) < limit {
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
	bi.Hash = m.selfHashHex
	bi.Network = m.network

	// Devices are persisted under the fat node's own libp2p peer ID by the
	// pair handler (s.Conn().LocalPeer()), not under the owner's user ID,
	// so look them up with the same key here.
	ownerPeerId := bi.ID.String()
	devices, err := m.deviceRepo.GetDevices(ownerPeerId)
	if err != nil {
		log.Infof("member: failed to get devices for owner %s: %s", ownerPeerId, err)
	}
	for _, device := range devices {
		bi.Aliases = append(bi.Aliases, device.NodeId)
	}
	return bi
}

func (m *MemberNode) SetNodePriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability) {
	m.node.Prioritizer().SetPriority(pid, r)
}

func (m *MemberNode) SetMaxNodePriority(pid warpnet.WarpPeerID) {
	m.node.Prioritizer().SetMaxPriority(pid)
}

func (m *MemberNode) SetMinNodePriority(pid warpnet.WarpPeerID) {
	m.node.Prioritizer().SetMinPriority(pid)
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
	if nodeIdStr == "" {
		return nil, fmt.Errorf("member: stream: %w", warpnet.ErrEmptyNodeId)
	}

	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	if nodeId == "" {
		return nil, fmt.Errorf("member: stream: %w: %s", warpnet.ErrMalformedNodeId, nodeIdStr)
	}

	var isMastodonID bool
	if m.pseudoNode != nil {
		isMastodonID = m.pseudoNode.IsMastodonID(nodeId)
	}

	if isMastodonID {
		log.Debugf("member: stream: peer %s is mastodon", nodeIdStr)
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

// memberRepos bundles every repo the member node's handlers need.
// Built once in setupHandlers and threaded through the per-feature
// handler-list builders below so the registration func itself stays
// small (golangci-lint maintidx).
type memberRepos struct {
	timelineRepo     *database.TimelineRepo
	tweetRepo        *database.TweetRepo
	replyRepo        *database.ReplyRepo
	likeRepo         *database.LikeRepo
	chatRepo         *database.ChatRepo
	mediaRepo        *database.MediaRepo
	notificationRepo *database.NotificationsRepo
	bookmarkRepo     *database.BookmarkRepo
	blocksRepo       *database.UserSetRepo
	mutesRepo        *database.UserSetRepo
	convMutesRepo    *database.ConvMuteRepo
	subsRepo         *database.UserSetRepo
	userNoteRepo     *database.UserNoteRepo
	tweetEditsRepo   *database.TweetEditsRepo
	convoRepo        *database.ConversationsRepo
}

func (m *MemberNode) setupHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
	db Storer,
	statsDB StatsStorer,
	privKey ed25519.PrivateKey,
) {
	if m == nil {
		panic("member: setup handlers: nil node")
	}

	r := &memberRepos{
		timelineRepo:     database.NewTimelineRepo(db),
		tweetRepo:        database.NewTweetRepo(db, statsDB),
		replyRepo:        database.NewRepliesRepo(db, statsDB),
		likeRepo:         database.NewLikeRepo(db, statsDB),
		chatRepo:         database.NewChatRepo(db),
		mediaRepo:        database.NewMediaRepo(db),
		notificationRepo: database.NewNotificationsRepo(db),
		bookmarkRepo:     database.NewBookmarkRepo(db),
		blocksRepo:       database.NewBlocksRepo(db),
		mutesRepo:        database.NewMutesRepo(db),
		convMutesRepo:    database.NewConvMutesRepo(db),
		subsRepo:         database.NewSubscriptionsRepo(db),
		userNoteRepo:     database.NewUserNoteRepo(db),
		tweetEditsRepo:   database.NewTweetEditsRepo(db),
		convoRepo:        database.NewConversationsRepo(db),
	}

	token := authRepo.SessionToken()

	hs := make([]warpnet.WarpStreamHandler, 0, 80)
	hs = append(hs, m.adminHandlers(token, privKey, db, authRepo, r)...)
	hs = append(hs, m.tweetHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.replyHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.engagementHandlers(userRepo, r)...)
	hs = append(hs, m.followHandlers(authRepo, userRepo, followRepo, r)...)
	hs = append(hs, m.userHandlers(authRepo, userRepo, followRepo, r)...)
	hs = append(hs, m.chatHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.mediaHandlers(userRepo, r)...)
	hs = append(hs, m.notificationHandlers(authRepo, r)...)
	hs = append(hs, m.socialFilterHandlers(userRepo, r)...)
	hs = append(hs, m.bookmarksAndConvosHandlers(r)...)

	m.node.SetStreamHandlers(hs...)
}

//nolint:govet
func (m *MemberNode) adminHandlers(
	token string,
	privKey ed25519.PrivateKey,
	db Storer,
	authRepo AuthProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_PAIR,
			handler.StreamNodesPairingHandler(token, m.deviceRepo),
		},
		{
			event.PUBLIC_POST_NODE_CHALLENGE,
			handler.StreamChallengeHandler(root.GetCodeBase(), privKey),
		},
		{
			event.PUBLIC_GET_INFO,
			handler.StreamGetInfoHandler(m, m.discService.DiscoveryHandlerStream),
		},
		{
			event.PRIVATE_GET_STATS,
			handler.StreamGetStatsHandler(m, db),
		},
		{
			event.PUBLIC_POST_MODERATION_RESULT,
			handler.StreamModerationResultHandler(r.notificationRepo, r.tweetRepo, authRepo, r.timelineRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) tweetHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_TIMELINE,
			handler.StreamTimelineHandler(r.timelineRepo),
		},
		{
			event.PRIVATE_POST_TWEET,
			handler.StreamNewTweetHandler(m.pubsubService, authRepo, r.tweetRepo, r.timelineRepo),
		},
		{
			event.PRIVATE_DELETE_TWEET,
			handler.StreamDeleteTweetHandler(m.pubsubService, authRepo, r.tweetRepo, r.timelineRepo, r.likeRepo),
		},
		{
			event.PUBLIC_GET_TWEETS,
			handler.StreamGetTweetsHandler(r.tweetRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET,
			handler.StreamGetTweetHandler(r.tweetRepo, authRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET_STATS,
			handler.StreamGetTweetStatsHandler(r.tweetRepo, r.likeRepo, r.tweetRepo, r.replyRepo, userRepo, m),
		},
		{
			event.PRIVATE_POST_TWEET_EDIT,
			handler.StreamEditTweetHandler(r.tweetRepo, r.tweetEditsRepo),
		},
		{
			event.PUBLIC_GET_TWEET_EDITS,
			handler.StreamGetTweetEditsHandler(r.tweetEditsRepo),
		},
		{
			event.PUBLIC_POST_PIN,
			handler.StreamPinTweetHandler(r.tweetRepo),
		},
		{
			event.PUBLIC_POST_UNPIN,
			handler.StreamUnpinTweetHandler(r.tweetRepo),
		},
		{
			event.PUBLIC_POST_RETWEET,
			handler.StreamNewReTweetHandler(userRepo, r.tweetRepo, r.timelineRepo, r.notificationRepo, m),
		},
		{
			event.PUBLIC_POST_UNRETWEET,
			handler.StreamUnretweetHandler(r.tweetRepo, userRepo, m),
		},
	}
}

//nolint:govet
func (m *MemberNode) replyHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_REPLY,
			handler.StreamNewReplyHandler(r.replyRepo, userRepo, r.notificationRepo, m, r.convoRepo),
		},
		{
			event.PUBLIC_DELETE_REPLY,
			handler.StreamDeleteReplyHandler(r.tweetRepo, userRepo, r.replyRepo, m),
		},
		{
			event.PUBLIC_GET_REPLY,
			handler.StreamGetReplyHandler(r.replyRepo, authRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_REPLIES,
			handler.StreamGetRepliesHandler(r.replyRepo, userRepo, m),
		},
	}
}

//nolint:govet
func (m *MemberNode) engagementHandlers(
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_LIKE,
			handler.StreamLikeHandler(r.likeRepo, userRepo, r.notificationRepo, m),
		},
		{
			event.PUBLIC_POST_UNLIKE,
			handler.StreamUnlikeHandler(r.likeRepo, userRepo, m),
		},
		{
			event.PUBLIC_POST_VIEW,
			handler.StreamViewHandler(r.tweetRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET_LIKERS,
			handler.StreamGetTweetLikersHandler(r.likeRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET_RETWEETERS,
			handler.StreamGetTweetRetweetersHandler(r.tweetRepo, userRepo, m),
		},
	}
}

//nolint:govet
func (m *MemberNode) followHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_FOLLOW,
			handler.StreamFollowHandler(m.pubsubService, followRepo, authRepo, userRepo, r.notificationRepo, m),
		},
		{
			event.PUBLIC_POST_IS_FOLLOWING,
			handler.StreamIsFollowingHandler(followRepo, authRepo),
		},
		{
			event.PUBLIC_POST_IS_FOLLOWER,
			handler.StreamIsFollowerHandler(followRepo, authRepo),
		},
		{
			event.PUBLIC_POST_UNFOLLOW,
			handler.StreamUnfollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_FOLLOWERS,
			handler.StreamGetFollowersHandler(authRepo, userRepo, followRepo, m),
		},
		{
			event.PUBLIC_GET_FOLLOWINGS,
			handler.StreamGetFollowingsHandler(authRepo, userRepo, followRepo, m),
		},
	}
}

//nolint:govet
func (m *MemberNode) userHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_GET_USER,
			handler.StreamGetUserHandler(r.tweetRepo, followRepo, userRepo, authRepo, m),
		},
		{
			event.PUBLIC_GET_USERS,
			handler.StreamGetUsersHandler(userRepo, m),
		},
		{
			event.PUBLIC_GET_USERS_SEARCH,
			handler.StreamSearchUsersHandler(userRepo),
		},
		{
			event.PUBLIC_GET_WHOTOFOLLOW,
			handler.StreamGetWhoToFollowHandler(authRepo, userRepo, followRepo),
		},
		{
			event.PRIVATE_POST_USER,
			handler.StreamUpdateProfileHandler(authRepo, userRepo),
		},
		{
			event.PRIVATE_POST_SUBSCRIBE_USER,
			handler.StreamSubscribeUserHandler(r.subsRepo),
		},
		{
			event.PRIVATE_POST_UNSUBSCRIBE_USER,
			handler.StreamUnsubscribeUserHandler(r.subsRepo),
		},
		{
			event.PRIVATE_POST_USER_NOTE,
			handler.StreamUpdateAccountNoteHandler(r.userNoteRepo),
		},
		{
			event.PRIVATE_GET_USER_NOTE,
			handler.StreamGetAccountNoteHandler(r.userNoteRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) chatHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_CHAT,
			handler.StreamCreateChatHandler(r.chatRepo, userRepo, m),
		},
		{
			event.PRIVATE_DELETE_CHAT,
			handler.StreamDeleteChatHandler(r.chatRepo, authRepo),
		},
		{
			event.PRIVATE_GET_CHATS,
			handler.StreamGetUserChatsHandler(r.chatRepo, authRepo),
		},
		{
			event.PUBLIC_POST_MESSAGE,
			handler.StreamNewMessageHandler(r.chatRepo, userRepo, m),
		},
		{
			event.PRIVATE_DELETE_MESSAGE,
			handler.StreamDeleteMessageHandler(r.chatRepo, authRepo),
		},
		{
			event.PRIVATE_GET_MESSAGE,
			handler.StreamGetMessageHandler(r.chatRepo, authRepo),
		},
		{
			event.PRIVATE_GET_MESSAGES,
			handler.StreamGetMessagesHandler(r.chatRepo, authRepo),
		},
		{
			event.PRIVATE_GET_CHAT,
			handler.StreamGetUserChatHandler(r.chatRepo, authRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) mediaHandlers(
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_UPLOAD_IMAGE,
			handler.StreamUploadImageHandler(m, r.mediaRepo, userRepo),
		},
		{
			event.PUBLIC_GET_IMAGE,
			handler.StreamGetImageHandler(m, r.mediaRepo, userRepo),
		},
		{
			event.PRIVATE_POST_MEDIA_META,
			handler.StreamUpdateMediaMetaHandler(r.mediaRepo),
		},
		{
			event.PRIVATE_GET_MEDIA,
			handler.StreamGetMediaHandler(r.mediaRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) notificationHandlers(
	authRepo AuthProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_NOTIFICATIONS,
			handler.StreamGetNotificationsHandler(r.notificationRepo, authRepo),
		},
		{
			event.PRIVATE_GET_NOTIFICATION,
			handler.StreamGetNotificationHandler(r.notificationRepo, authRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) socialFilterHandlers(
	userRepo UserProvider,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_BLOCK,
			handler.StreamBlockHandler(r.blocksRepo, userRepo, m.nodeRepo),
		},
		{
			event.PRIVATE_POST_UNBLOCK,
			handler.StreamUnblockHandler(r.blocksRepo),
		},
		{
			event.PRIVATE_GET_BLOCKS,
			handler.StreamGetBlocksHandler(r.blocksRepo),
		},
		{
			event.PRIVATE_POST_MUTE,
			handler.StreamMuteHandler(r.mutesRepo),
		},
		{
			event.PRIVATE_POST_UNMUTE,
			handler.StreamUnmuteHandler(r.mutesRepo),
		},
		{
			event.PRIVATE_GET_MUTES,
			handler.StreamGetMutesHandler(r.mutesRepo),
		},
		{
			event.PRIVATE_POST_MUTE_CONVERSATION,
			handler.StreamMuteConversationHandler(r.convMutesRepo),
		},
		{
			event.PRIVATE_POST_UNMUTE_CONVERSATION,
			handler.StreamUnmuteConversationHandler(r.convMutesRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) bookmarksAndConvosHandlers(r *memberRepos) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_BOOKMARK,
			handler.StreamBookmarkHandler(r.bookmarkRepo),
		},
		{
			event.PRIVATE_POST_UNBOOKMARK,
			handler.StreamUnbookmarkHandler(r.bookmarkRepo),
		},
		{
			event.PRIVATE_GET_BOOKMARKS,
			handler.StreamGetBookmarksHandler(r.bookmarkRepo),
		},
		{
			event.PRIVATE_GET_CONVERSATIONS,
			handler.StreamGetConversationsHandler(r.convoRepo),
		},
		{
			event.PRIVATE_DELETE_CONVERSATION,
			handler.StreamDeleteConversationHandler(r.convoRepo),
		},
	}
}

func (m *MemberNode) SetStreamHandlers(hs ...warpnet.WarpStreamHandler) {
	m.node.SetStreamHandlers(hs...)
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
	if m.statsDb != nil {
		_ = m.statsDb.Close()
	}

	if m.nodeRepo != nil {
		if err := m.nodeRepo.Close(); err != nil {
			log.Errorf("member: failed to close node repo: %v", err)
		}
	}
	m.node.StopNode()
}
