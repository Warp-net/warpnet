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

	memberPubSub "github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/middleware"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/notifications"
	"github.com/Warp-net/warpnet/core/rating"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
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
	mw   *middleware.WarpMiddleware

	discService      DiscoveryHandler
	mdnsService      MDNSStarterCloser
	pubsubService    PubSubProvider
	dHashTable       DistributedHashTableCloser
	nodeRepo         NodeProvider
	statsRepo        StatsProvider
	rating           *rating.Handle
	ratingDb         RatingStorer
	authRepo         AuthProvider
	userRepo         UserProvider
	aliasesRepo      AliasesProvider
	followRepo       FollowStorer
	notifier         notifications.Notifier
	db               Storer
	crdtDb           *crdt.Store
	statsDb          StatsStorer
	privKey          ed25519.PrivateKey
	metrics          MetricsOnlinePusher
	ownerId, network string
}

func NewMemberNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
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
	followRepo := database.NewFollowRepo(db)
	aliasesRepo := database.NewAliasesRepo(db)
	owner := authRepo.GetOwner()

	// Apply the owner's configured ActivityPub gateway id (empty falls back to
	// the built-in default) before seeding the entry user and starting discovery.
	if gw, err := database.NewSettingsRepo(db).GetGatewaySettings(owner.UserId); err == nil {
		mastodon.SetGatewayNodeID(gw.NodeID)
	}

	// Seed the mastodon gateway user with a plain repo so it doesn't notify.
	mastodon.SeedEntryUser(database.NewUserRepo(db))

	notifier := notifications.New(
		notifications.NewStoreChannel(database.NewNotificationsRepo(db)),
		notifications.NewEmailChannel(database.NewSettingsRepo(db), notifications.NewSMTPMailer()),
	)
	userRepo := database.NewUserRepoNotifying(db, notifier, owner.UserId)

	ratingHandle := rating.NewHandle()
	discService := discovery.NewDiscoveryService(ctx, userRepo, nodeRepo, metrics, ratingHandle)
	mdnsService := mdns.NewMulticastDNS(ctx, discService.DiscoveryHandlerMDNS)

	followingIds, err := fetchFollowingIds(owner.UserId, followRepo)
	if err != nil {
		return nil, err
	}

	pubSubHandlers := memberPubSub.PrefollowHandlers(followingIds...)
	pubSubHandlers = append(
		pubSubHandlers,
		memberPubSub.NewRelayDiscoveryTopicHandler(discService.DiscoveryHandlerPubSub),
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
		rating:        ratingHandle,
		discService:   discService,
		mdnsService:   mdnsService,
		pubsubService: pubsubService,
		dHashTable:    dHashTable,
		nodeRepo:      nodeRepo,
		statsRepo:     statsRepo,
		userRepo:      userRepo,
		followRepo:    followRepo,
		aliasesRepo:   aliasesRepo,
		authRepo:      authRepo,
		notifier:      notifier,
		db:            db,
		privKey:       privKey,
		metrics:       metrics,
		ownerId:       owner.UserId,
		network:       warpNetwork,
	}

	return mn, nil
}

func (m *MemberNode) Start() (err error) {
	m.node, err = node.NewWarpNode(
		m.ctx,
		m.rating,
		m.opts...,
	)
	if err != nil {
		return fmt.Errorf("member: failed to start node: %w", err)
	}

	m.node.SetOutbox(database.NewOutboxRepo(m.db))

	m.pubsubService.Gossip().SetRating(m.rating)
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
	m.crdtDb, err = crdt.NewStore(
		m.ctx, crdtBroadcaster, m.statsRepo, m.node.Node(), m.dHashTable,
	)
	if err != nil {
		return fmt.Errorf("member: failed to initialize crdt datastore: %w", err)
	}

	m.statsDb, err = crdt.NewCRDTStatsStore(m.ctx, m.crdtDb, m.node.Node())
	if err != nil {
		return fmt.Errorf("member: failed to initialize stats store: %w", err)
	}

	store, err := rating.NewNodeStore(m.ctx, m.crdtDb, m.node.Node(), m.privKey, warpnet.MemberNode)
	if err != nil {
		log.Errorf("member: failed to initialize rating store: %v", err)
	} else {
		m.ratingDb = store
		m.rating.Set(store)
	}

	m.mw = middleware.NewWarpMiddleware(m.node.Node().ID(), m.aliasesRepo, m.rating)
	m.node.SetStreamMiddlewares(
		m.mw.LoggingMiddleware,
		m.mw.RateLimiterMiddleware,
		m.mw.AuthMiddleware,
		m.mw.IdempotencyMiddleware,
	)

	m.setupHandlers(m.authRepo, m.userRepo, m.followRepo, m.db, m.statsDb)

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

	return m.node.Connect(p)
}

func (m *MemberNode) NodeInfo() warpnet.NodeInfo {
	bi := m.node.BaseNodeInfo()
	bi.OwnerId = m.ownerId
	bi.Network = m.network
	bi.Type = warpnet.MemberNode

	// Devices are persisted under the fat node's own libp2p peer ID by the
	// pair handler (s.Conn().LocalPeer()), not under the owner's user ID,
	// so look them up with the same key here.
	ownerPeerId := bi.ID.String()
	aliases, err := m.aliasesRepo.GetAliases()
	if err != nil {
		log.Infof("member: failed to get devices for owner %s: %s", ownerPeerId, err)
	}
	for _, alias := range aliases {
		bi.Aliases = append(bi.Aliases, warpnet.WarpPeerID(alias.NodeId))
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

func (m *MemberNode) SelfStream(
	from, to warpnet.WarpPeerID, path stream.WarpRoute, data any,
) (_ []byte, err error) {
	if m == nil || m.node == nil {
		return nil, nil
	}
	return m.node.SelfStream(from, to, path, data)
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

	bt, err := m.node.Stream(nodeId, path, data)
	if errors.Is(err, warpnet.ErrNodeIsOffline) {
		m.setUserOffline(nodeIdStr)
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
	if u.IsOffline {
		return
	}
	u.IsOffline = true
	_, err = m.userRepo.Update(u.Id, u)
	// The flag is monotonic: a commit conflict means a concurrent
	// stream failure already stored the same thing — not an error.
	if err != nil && !errors.Is(err, database.ErrConflict) {
		log.Warningf("member: stream: failed to set user offline: %v", err)
	}
}

// memberRepos bundles every repo the member node's handlers need.
// Built once in setupHandlers and threaded through the per-feature
// handler-list builders below so the registration func itself stays
// small (golangci-lint maintidx).
type memberRepos struct {
	timelineRepo     TimelineProvider
	tweetRepo        TweetsProvider
	reactionRepo     ReactionsProvider
	pollRepo         PollProvider
	chatRepo         ChatProvider
	mediaRepo        MediaProvider
	notificationRepo NotificationProvider
	settingsRepo     SettingsProvider
	bookmarkRepo     BookmarkProvider
	blocksRepo       BlocksProvider
	mutesRepo        MutesProvider
	subsRepo         SubscriptionProvider
	filterRepo       FilterProvider
}

func (m *MemberNode) setupHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
	db Storer,
	statsDB StatsStorer,
) {
	if m == nil {
		panic("member: setup handlers: nil node")
	}

	r := &memberRepos{
		timelineRepo:     database.NewTimelineRepo(db),
		tweetRepo:        database.NewTweetRepo(db, statsDB),
		reactionRepo:     database.NewReactionRepo(db, statsDB),
		pollRepo:         database.NewPollRepo(db, statsDB),
		chatRepo:         database.NewChatRepo(db),
		mediaRepo:        database.NewMediaRepo(db),
		notificationRepo: database.NewNotificationsRepo(db),
		settingsRepo:     database.NewSettingsRepo(db),
		bookmarkRepo:     database.NewBookmarkRepo(db),
		blocksRepo:       database.NewBlocksRepo(db),
		mutesRepo:        database.NewMutesRepo(db),
		subsRepo:         database.NewSubscriptionsRepo(db),
		filterRepo:       database.NewFilterRepo(db),
	}

	hs := make([]warpnet.WarpStreamHandler, 0, 80)
	hs = append(hs, m.adminHandlers(authRepo, db, r)...)
	hs = append(hs, m.tweetHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.engagementHandlers(userRepo, r)...)
	hs = append(hs, m.followHandlers(authRepo, userRepo, followRepo)...)
	hs = append(hs, m.followRequestHandlers(followRepo)...)
	hs = append(hs, m.filterHandlers(r)...)
	hs = append(hs, m.userHandlers(authRepo, userRepo, followRepo, r)...)
	hs = append(hs, m.chatHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.mediaHandlers(userRepo, r)...)
	hs = append(hs, m.notificationHandlers(authRepo, r)...)
	hs = append(hs, m.settingsHandlers(authRepo, r)...)
	hs = append(hs, m.socialFilterHandlers(userRepo, r)...)
	hs = append(hs, m.bookmarksHandlers(r)...)

	m.node.SetStreamHandlers(hs...)
}

//nolint:govet
func (m *MemberNode) adminHandlers(
	authRepo AuthProvider,
	db Storer,
	r *memberRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_PAIR,
			handler.StreamNodesPairingHandler(authRepo, m.aliasesRepo, m),
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
			event.PRIVATE_GET_RATING,
			handler.StreamGetOwnRatingHandler(m.ratingDb),
		},
		{
			event.PUBLIC_GET_RATING,
			handler.StreamGetRatingHandler(m.ratingDb),
		},
		{
			event.PUBLIC_POST_MODERATION_RESULT,
			handler.StreamModerationResultHandler(
				m.notifier, r.tweetRepo, m.userRepo, r.timelineRepo, authRepo, m.rating,
			),
		},
		{
			event.PUBLIC_POST_REPORT,
			handler.StreamReportHandler(m.pubsubService),
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
			handler.StreamNewTweetHandler(m.pubsubService, authRepo, r.tweetRepo, r.timelineRepo, m.followRepo, userRepo, m.notifier, m),
		},
		{
			event.PUBLIC_POST_REPLY,
			handler.StreamNewReplyHandler(r.tweetRepo, userRepo, m.notifier, m),
		},
		{
			event.PRIVATE_POST_IMPORT_TWITTER_TWEET,
			handler.StreamImportTweetHandler(m, m.privKey, r.tweetRepo, r.mediaRepo, userRepo),
		},
		{
			event.PRIVATE_DELETE_TWEET,
			handler.StreamDeleteTweetHandler(m.pubsubService, authRepo, r.tweetRepo, r.timelineRepo, r.reactionRepo, m),
		},
		{
			event.PUBLIC_POST_TIMELINE,
			handler.StreamTimelineNewTweetHandler(authRepo, r.tweetRepo, r.timelineRepo, m.followRepo, userRepo),
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
			handler.StreamGetTweetStatsHandler(r.tweetRepo, r.reactionRepo, r.tweetRepo, r.tweetRepo, userRepo, m),
		},
		{
			event.PRIVATE_POST_TWEET_EDIT,
			handler.StreamEditTweetHandler(r.tweetRepo, r.timelineRepo),
		},
		{
			event.PUBLIC_POST_PIN,
			handler.StreamPinTweetHandler(r.tweetRepo, userRepo),
		},
		{
			event.PUBLIC_POST_UNPIN,
			handler.StreamUnpinTweetHandler(r.tweetRepo, userRepo),
		},
		{
			event.PUBLIC_POST_RETWEET,
			handler.StreamNewReTweetHandler(userRepo, r.tweetRepo, r.timelineRepo, m.notifier, m),
		},
		{
			event.PUBLIC_POST_UNRETWEET,
			handler.StreamUnretweetHandler(r.tweetRepo, userRepo, m),
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
			event.PUBLIC_POST_REACT,
			handler.StreamReactionHandler(r.reactionRepo, userRepo, m.notifier, m),
		},
		{
			event.PUBLIC_POST_UNREACT,
			handler.StreamUnreactionHandler(r.reactionRepo, userRepo, m),
		},
		{
			event.PUBLIC_POST_VIEW,
			handler.StreamViewHandler(r.tweetRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET_REACTORS,
			handler.StreamGetTweetReactorsHandler(r.reactionRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_TWEET_RETWEETERS,
			handler.StreamGetTweetRetweetersHandler(r.tweetRepo, userRepo, m),
		},
		{
			event.PRIVATE_GET_REACTIONS,
			handler.StreamGetReactionsHandler(r.reactionRepo),
		},
		{
			event.PUBLIC_POST_POLL_VOTE,
			handler.StreamPollVoteHandler(r.pollRepo, r.tweetRepo, userRepo, m),
		},
		{
			event.PUBLIC_GET_POLL,
			handler.StreamGetPollHandler(r.pollRepo, r.tweetRepo, userRepo, m),
		},
	}
}

//nolint:govet
func (m *MemberNode) followHandlers(
	authRepo AuthProvider,
	userRepo UserProvider,
	followRepo FollowStorer,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_FOLLOW,
			handler.StreamFollowHandler(m.pubsubService, followRepo, authRepo, userRepo, m.notifier, m),
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
func (m *MemberNode) followRequestHandlers(
	followRepo FollowStorer,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_FOLLOW_REQUESTS,
			handler.StreamGetFollowRequestsHandler(followRepo),
		},
		{
			event.PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE,
			handler.StreamAuthorizeFollowRequestHandler(followRepo),
		},
		{
			event.PRIVATE_POST_FOLLOW_REQUEST_REJECT,
			handler.StreamRejectFollowRequestHandler(followRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) filterHandlers(r *memberRepos) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_FILTER,
			handler.StreamGetFilterHandler(r.filterRepo),
		},
		{
			event.PRIVATE_GET_FILTERS,
			handler.StreamGetFiltersHandler(r.filterRepo),
		},
		{
			event.PRIVATE_POST_FILTER,
			handler.StreamNewFilterHandler(r.filterRepo),
		},
		{
			event.PRIVATE_POST_FILTER_UPDATE,
			handler.StreamUpdateFilterHandler(r.filterRepo),
		},
		{
			event.PRIVATE_DELETE_FILTER,
			handler.StreamDeleteFilterHandler(r.filterRepo),
		},
		{
			event.PRIVATE_POST_FILTER_KEYWORD,
			handler.StreamAddFilterKeywordHandler(r.filterRepo),
		},
		{
			event.PRIVATE_POST_FILTER_KEYWORD_UPDATE,
			handler.StreamUpdateFilterKeywordHandler(r.filterRepo),
		},
		{
			event.PRIVATE_DELETE_FILTER_KEYWORD,
			handler.StreamDeleteFilterKeywordHandler(r.filterRepo),
		},
	}
}

//nolint:govet
func (m *MemberNode) settingsHandlers(authRepo AuthProvider, r *memberRepos) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_NOTIFICATION_SETTINGS,
			handler.StreamGetNotificationSettingsHandler(r.settingsRepo, authRepo),
		},
		{
			event.PRIVATE_POST_NOTIFICATION_SETTINGS,
			handler.StreamUpdateNotificationSettingsHandler(r.settingsRepo, authRepo),
		},
		{
			event.PRIVATE_GET_GATEWAY_SETTINGS,
			handler.StreamGetGatewaySettingsHandler(r.settingsRepo, authRepo),
		},
		{
			event.PRIVATE_POST_GATEWAY_SETTINGS,
			handler.StreamUpdateGatewaySettingsHandler(r.settingsRepo, authRepo),
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
			handler.StreamNewMessageHandler(r.chatRepo, userRepo, m.notifier, m),
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
			handler.StreamUploadImageHandler(m, m.privKey, r.mediaRepo, userRepo),
		},
		{
			event.PUBLIC_GET_IMAGE,
			handler.StreamGetImageHandler(m, r.mediaRepo, userRepo),
		},
		{
			event.PRIVATE_POST_UPLOAD_VIDEO,
			handler.StreamUploadVideoHandler(m, m.privKey, r.mediaRepo, userRepo),
		},
		{
			event.PUBLIC_GET_VIDEO,
			handler.StreamGetVideoHandler(m, r.mediaRepo, userRepo),
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
			event.PRIVATE_GET_PUSHES,
			handler.StreamGetPushesHandler(r.notificationRepo, authRepo),
		},
		{
			event.PRIVATE_GET_NOTIFICATION,
			handler.StreamGetNotificationHandler(r.notificationRepo, authRepo),
		},
		{
			event.PRIVATE_POST_NOTIFICATION_READ,
			handler.StreamMarkNotificationReadHandler(r.notificationRepo, authRepo),
		},
		{
			event.PRIVATE_POST_NOTIFICATIONS_READ,
			handler.StreamMarkAllNotificationsReadHandler(r.notificationRepo, authRepo),
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
			handler.StreamUnblockHandler(r.blocksRepo, userRepo, m.nodeRepo),
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
	}
}

//nolint:govet
func (m *MemberNode) bookmarksHandlers(r *memberRepos) []warpnet.WarpStreamHandler {
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

func (m *MemberNode) PublicAddrs() []warpnet.WarpAddress {
	if m == nil || m.node == nil {
		return nil
	}

	publicAddrs := make([]warpnet.WarpAddress, 0, len(m.node.Node().Addrs()))
	for _, ma := range m.node.Node().Addrs() {
		if warpnet.IsPublicMultiAddress(ma) || warpnet.IsRelayMultiaddress(ma) {
			publicAddrs = append(publicAddrs, ma)
		}
	}
	return publicAddrs
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
	if m.ratingDb != nil {
		_ = m.ratingDb.Close()
	}
	if m.crdtDb != nil {
		_ = m.crdtDb.Close()
	}

	if m.nodeRepo != nil {
		if err := m.nodeRepo.Close(); err != nil {
			log.Errorf("member: failed to close node repo: %v", err)
		}
	}
	if m.mw != nil {
		m.mw.Close()
	}
	m.node.StopNode()
}
