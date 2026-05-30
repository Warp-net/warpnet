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

// Package node hosts the business node. It mirrors the member node
// (cmd/node/member/node) — same discovery, DHT, MDNS, pubsub and the full
// member-style handler set so a business profile and its posts are queryable
// like any other user — and adds three business obligations on top:
//
//   - NodeInfo.Role = "business" so peers learn the kind over the wire;
//   - a public-reachability assertion (panic if libp2p settles on
//     ReachabilityPrivate) since a business node must be publicly addressable;
//   - the moderator engine + report subscription, wired onto the same gossip
//     instance the node already runs (see moderation.go).
//
// Member node code is reused as a library (handlers, pubsub, discovery) and is
// not modified.
package node

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"
	root "github.com/Warp-net/warpnet"
	membernode "github.com/Warp-net/warpnet/cmd/node/member/node"
	memberPubSub "github.com/Warp-net/warpnet/cmd/node/member/pubsub"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/challenge"
	"github.com/Warp-net/warpnet/core/crdt"
	"github.com/Warp-net/warpnet/core/dht"
	"github.com/Warp-net/warpnet/core/discovery"
	"github.com/Warp-net/warpnet/core/handler"
	"github.com/Warp-net/warpnet/core/mdns"
	"github.com/Warp-net/warpnet/core/node"
	corePubsub "github.com/Warp-net/warpnet/core/pubsub"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/retrier"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	log "github.com/sirupsen/logrus"
)

type BusinessNode struct {
	ctx context.Context

	node *node.WarpNode
	opts []warpnet.WarpOption

	discService                   membernode.DiscoveryHandler
	mdnsService                   membernode.MDNSStarterCloser
	pubsubService                 *memberPubSub.MemberPubSub
	dHashTable                    membernode.DistributedHashTableCloser
	nodeRepo                      membernode.NodeProvider
	statsRepo                     membernode.StatsProvider
	authRepo                      membernode.AuthProvider
	userRepo                      membernode.UserProvider
	deviceRepo                    membernode.DeviceProvider
	followRepo                    membernode.FollowStorer
	db                            membernode.Storer
	statsDb                       membernode.StatsStorer
	privKey                       ed25519.PrivateKey
	ownerId, selfHashHex, network string
	retrier                       retrier.Retrier

	// moder is the moderator engine wrapper, set by StartModerator. Held as a
	// closer so this file needn't import the moderator package.
	moder interface{ Close() }
}

func NewBusinessNode(
	ctx context.Context,
	privKey ed25519.PrivateKey,
	psk security.PSK,
	ownNodeId warpnet.WarpPeerID,
	selfHashHex string,
	version *semver.Version,
	authRepo membernode.AuthProvider,
	db membernode.Storer,
	bootstrapNodes []warpnet.WarpAddrInfo,
	metrics membernode.MetricsOnlinePusher,
) (_ *BusinessNode, err error) {
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

	bn := &BusinessNode{
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
		network:       warpNetwork,
	}

	return bn, nil
}

func (m *BusinessNode) Start() (err error) {
	m.node, err = node.NewWarpNode(
		m.ctx,
		m.opts...,
	)
	if err != nil {
		return fmt.Errorf("business: failed to start node: %w", err)
	}

	m.pubsubService.Run(m)
	if err := m.discService.Run(m); err != nil {
		return err
	}

	m.mdnsService.Start(m)

	nodeInfo := m.NodeInfo()

	crdtBroadcaster, err := crdt.NewGossipBroadcaster(m.ctx, m.pubsubService.Gossip())
	if err != nil {
		return fmt.Errorf("business: failed to start crdt gossip broadcaster: %w", err)
	}
	m.statsDb, err = crdt.NewCRDTStatsStore(
		m.ctx, crdtBroadcaster, m.statsRepo, m.node.Node(), m.dHashTable,
	)
	if err != nil {
		return fmt.Errorf("business: failed to initialize stats store: %w", err)
	}

	m.setupHandlers(m.authRepo, m.userRepo, m.followRepo, m.db, m.statsDb, m.privKey)

	for _, addr := range m.dHashTable.BootstrapNodes() {
		m.SetMaxNodePriority(addr.ID)
	}

	println()
	fmt.Printf(
		"\033[1mBUSINESS NODE STARTED WITH ID %s AND ADDRESSES %v\033[0m\n",
		nodeInfo.ID.String(), nodeInfo.Addresses,
	)
	println()
	return nil
}

func fetchFollowingIds(ownerId string, followRepo membernode.FollowStorer) (ids []string, err error) {
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

func (m *BusinessNode) Connect(p warpnet.WarpAddrInfo) error {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Connect(p)
}

// NodeInfo carries the business owner's real user id (so the node is cached
// and queried like any member) plus Role = "business", the discriminator a
// peer learns through PUBLIC_GET_INFO.
func (m *BusinessNode) NodeInfo() warpnet.NodeInfo {
	bi := m.node.BaseNodeInfo()
	bi.OwnerId = m.ownerId
	bi.Hash = m.selfHashHex
	bi.Network = m.network
	bi.Role = warpnet.BusinessRole

	ownerPeerId := bi.ID.String()
	devices, err := m.deviceRepo.GetDevices(ownerPeerId)
	if err != nil {
		log.Infof("business: failed to get devices for owner %s: %s", ownerPeerId, err)
	}
	for _, device := range devices {
		bi.Aliases = append(bi.Aliases, device.NodeId)
	}
	return bi
}

// ID is required by the moderator's ModeratorNode interface.
func (m *BusinessNode) ID() warpnet.WarpPeerID {
	return m.node.Node().ID()
}

// Gossip exposes the single gossipsub instance the node runs so the moderator
// report subscription can attach to it (a host can only host one gossipsub
// router, so the moderator must share the node's own).
func (m *BusinessNode) Gossip() *corePubsub.Gossip {
	if m == nil || m.pubsubService == nil {
		return nil
	}
	return m.pubsubService.Gossip()
}

func (m *BusinessNode) Reachability() warpnet.WarpReachability {
	return m.NodeInfo().Reachability
}

// AssertPublicReachability enforces the business node's public-IP obligation.
// It waits out an initial grace window (AutoNAT v2 needs several probe dials to
// settle and reports Unknown/Private transiently at boot), then samples
// reachability. It returns as soon as a Public reading is seen, and panics only
// after several consecutive Private readings — the flapping protection that
// keeps a single bad probe from killing the node. A still-Unknown verdict past
// the max wait is left as a warning rather than a panic. Intended to run on its
// own goroutine; the panic crashes the process, which is the assertion.
func (m *BusinessNode) AssertPublicReachability() {
	const (
		grace         = 90 * time.Second
		sampleEvery   = 5 * time.Second
		privateStreak = 3
		maxWait       = 5 * time.Minute
	)

	select {
	case <-m.ctx.Done():
		return
	case <-time.After(grace):
	}

	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(sampleEvery)
	defer ticker.Stop()

	streak := 0
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			switch m.Reachability() {
			case warpnet.ReachabilityPublic:
				log.Infoln("business: reachability confirmed public")
				return
			case warpnet.ReachabilityPrivate:
				streak++
				log.Warnf("business: reachability reported private (%d/%d)", streak, privateStreak)
				if streak >= privateStreak {
					panic("business: node is privately reachable (behind NAT) — a business node must have a publicly addressable IP")
				}
			default:
				streak = 0
			}
			if time.Now().After(deadline) {
				log.Warnln("business: reachability still unknown after max wait; continuing without public confirmation")
				return
			}
		}
	}
}

func (m *BusinessNode) SetNodePriority(pid warpnet.WarpPeerID, r warpnet.WarpReachability) {
	m.node.Prioritizer().SetPriority(pid, r)
}

func (m *BusinessNode) SetMaxNodePriority(pid warpnet.WarpPeerID) {
	m.node.Prioritizer().SetMaxPriority(pid)
}

func (m *BusinessNode) SetMinNodePriority(pid warpnet.WarpPeerID) {
	m.node.Prioritizer().SetMinPriority(pid)
}

func (m *BusinessNode) SelfStream(path stream.WarpRoute, data any) (_ []byte, err error) {
	if m == nil || m.node == nil {
		return nil, nil
	}
	return m.node.SelfStream(path, data)
}

func (m *BusinessNode) GenericStream(nodeIdStr string, path stream.WarpRoute, data any) (_ []byte, err error) {
	if m == nil {
		return nil, nil
	}
	if nodeIdStr == "" {
		return nil, fmt.Errorf("business: stream: %w", warpnet.ErrEmptyNodeId)
	}

	nodeId := warpnet.FromStringToPeerID(nodeIdStr)
	if nodeId == "" {
		return nil, fmt.Errorf("business: stream: %w: %s", warpnet.ErrMalformedNodeId, nodeIdStr)
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
			bt, err = m.node.Stream(nodeId, path, data)
			return err
		})
	}
	return bt, err
}

func (m *BusinessNode) setUserOffline(nodeIdStr string) {
	if m == nil {
		return
	}
	u, err := m.userRepo.GetByNodeID(nodeIdStr)
	if errors.Is(err, database.ErrUserNotFound) {
		return
	}
	if err != nil {
		log.Warningf("business: stream: failed to get user: %v", err)
		return
	}
	u.IsOffline = true
	_, err = m.userRepo.Update(u.Id, u)
	if err != nil {
		log.Warningf("business: stream: failed to set user offline: %v", err)
		return
	}
}

// businessRepos bundles every repo the handlers need. Built once in
// setupHandlers and threaded through the per-feature builders so the
// registration func stays small (golangci-lint maintidx).
type businessRepos struct {
	timelineRepo     *database.TimelineRepo
	tweetRepo        *database.TweetRepo
	replyRepo        *database.ReplyRepo
	likeRepo         *database.LikeRepo
	chatRepo         *database.ChatRepo
	mediaRepo        *database.MediaRepo
	notificationRepo *database.NotificationsRepo
	bookmarkRepo     *database.BookmarkRepo
	blocksRepo       *database.BlocksRepo
	mutesRepo        *database.MutesRepo
	subsRepo         *database.SubscriptionsRepo
	filterRepo       *database.FilterRepo
}

func (m *BusinessNode) setupHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	followRepo membernode.FollowStorer,
	db membernode.Storer,
	statsDB membernode.StatsStorer,
	privKey ed25519.PrivateKey,
) {
	if m == nil {
		panic("business: setup handlers: nil node")
	}

	r := &businessRepos{
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
		subsRepo:         database.NewSubscriptionsRepo(db),
		filterRepo:       database.NewFilterRepo(db),
	}

	token := authRepo.SessionToken()

	hs := make([]warpnet.WarpStreamHandler, 0, 80)
	hs = append(hs, m.adminHandlers(token, privKey, db, r)...)
	hs = append(hs, m.tweetHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.replyHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.engagementHandlers(userRepo, r)...)
	hs = append(hs, m.followHandlers(authRepo, userRepo, followRepo, r)...)
	hs = append(hs, m.followRequestHandlers(followRepo)...)
	hs = append(hs, m.filterHandlers(r)...)
	hs = append(hs, m.userHandlers(authRepo, userRepo, followRepo, r)...)
	hs = append(hs, m.chatHandlers(authRepo, userRepo, r)...)
	hs = append(hs, m.mediaHandlers(userRepo, r)...)
	hs = append(hs, m.notificationHandlers(authRepo, r)...)
	hs = append(hs, m.socialFilterHandlers(userRepo, r)...)
	hs = append(hs, m.bookmarksHandlers(r)...)

	m.node.SetStreamHandlers(hs...)
}

//nolint:govet
func (m *BusinessNode) adminHandlers(
	token string,
	privKey ed25519.PrivateKey,
	db membernode.Storer,
	r *businessRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_POST_PAIR,
			handler.StreamNodesPairingHandler(token, m.deviceRepo, m),
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
			handler.StreamModerationResultHandler(r.tweetRepo, m.userRepo, r.timelineRepo),
		},
		{
			event.PUBLIC_POST_REPORT,
			handler.StreamReportHandler(m.pubsubService),
		},
	}
}

//nolint:govet
func (m *BusinessNode) tweetHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	r *businessRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PRIVATE_GET_TIMELINE,
			handler.StreamTimelineHandler(r.timelineRepo),
		},
		{
			event.PRIVATE_POST_TWEET,
			handler.StreamNewTweetHandler(m.pubsubService, authRepo, r.tweetRepo, r.timelineRepo, m.followRepo),
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
			handler.StreamEditTweetHandler(r.tweetRepo, r.timelineRepo),
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
func (m *BusinessNode) replyHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	r *businessRepos,
) []warpnet.WarpStreamHandler {
	return []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_REPLY,
			handler.StreamNewReplyHandler(r.replyRepo, userRepo, r.notificationRepo, m),
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
			handler.StreamGetRepliesHandler(r.replyRepo),
		},
	}
}

//nolint:govet
func (m *BusinessNode) engagementHandlers(
	userRepo membernode.UserProvider,
	r *businessRepos,
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
func (m *BusinessNode) followHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	followRepo membernode.FollowStorer,
	r *businessRepos,
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
func (m *BusinessNode) followRequestHandlers(
	followRepo membernode.FollowStorer,
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
func (m *BusinessNode) filterHandlers(r *businessRepos) []warpnet.WarpStreamHandler {
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
func (m *BusinessNode) userHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	followRepo membernode.FollowStorer,
	r *businessRepos,
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
func (m *BusinessNode) chatHandlers(
	authRepo membernode.AuthProvider,
	userRepo membernode.UserProvider,
	r *businessRepos,
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
func (m *BusinessNode) mediaHandlers(
	userRepo membernode.UserProvider,
	r *businessRepos,
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
func (m *BusinessNode) notificationHandlers(
	authRepo membernode.AuthProvider,
	r *businessRepos,
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
		{
			event.PRIVATE_POST_NOTIFICATION_READ,
			handler.StreamMarkNotificationReadHandler(r.notificationRepo, authRepo),
		},
	}
}

//nolint:govet
func (m *BusinessNode) socialFilterHandlers(
	userRepo membernode.UserProvider,
	r *businessRepos,
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
func (m *BusinessNode) bookmarksHandlers(r *businessRepos) []warpnet.WarpStreamHandler {
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

func (m *BusinessNode) SetStreamHandlers(hs ...warpnet.WarpStreamHandler) {
	m.node.SetStreamHandlers(hs...)
}

func (m *BusinessNode) Node() warpnet.P2PNode {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node()
}

func (m *BusinessNode) Peerstore() warpnet.WarpPeerstore {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node().Peerstore()
}

func (m *BusinessNode) Network() warpnet.WarpNetwork {
	if m == nil || m.node == nil {
		return nil
	}
	return m.node.Node().Network()
}

func (m *BusinessNode) PublicAddrs() []warpnet.WarpAddress {
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

func (m *BusinessNode) SimpleConnect(info warpnet.WarpAddrInfo) error {
	return m.node.Node().Connect(m.ctx, info)
}

func (m *BusinessNode) Stop() {
	if m == nil {
		return
	}
	if m.moder != nil {
		m.moder.Close()
	}
	if m.discService != nil {
		m.discService.Close()
	}
	if m.mdnsService != nil {
		m.mdnsService.Close()
	}
	if m.pubsubService != nil {
		if err := m.pubsubService.Close(); err != nil {
			log.Errorf("business: failed to close pubsub: %v", err)
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
			log.Errorf("business: failed to close node repo: %v", err)
		}
	}
	if m.node != nil {
		m.node.StopNode()
	}
}
