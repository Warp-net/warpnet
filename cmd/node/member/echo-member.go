//go:build echo && !remote

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

//nolint:all
package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

const (
	usernamePrefix = "Echo"
	echoPassword   = `\@4o97Z7<Cfu`
	// 23 chars: ECHO_INDEX fills the remaining three of the 26-char ULID.
	echoOwnerPrefix = "01KSGHBHKG0N77T6A3RZV8W"

	echoReplyPrefix   = "echo: "
	echoChatReply     = "echo: received message"
	messageLimit      = 5000
	seenTTL           = 999 * time.Minute
	pruneInterval     = 999 * time.Minute
	maxSeenKeys       = 10_000
	ownTweetInterval  = 24 * time.Hour
	ownTweetFallback  = "echo: hello from the warpnet — random Chuck quote API was unavailable"
	ownTweetCharLimit = 4096
)

// The node key comes from the account, not from NODE_SEED: DeriveIdentityKey
// hashes username, password and network, so echo nodes sharing a username also
// share a peer ID. ECHO_INDEX is what keeps a group of them apart.
var (
	echoIndex   = echoIndexFromEnv()
	username    = fmt.Sprintf("%s%d", usernamePrefix, echoIndex)
	echoOwnerID = fmt.Sprintf("%s%03d", echoOwnerPrefix, echoIndex)
)

// Auto-interaction stays off unless WARP_LOADTEST is set, so an echo node that
// is not part of a run keeps answering PRIVATE_POST_TWEET with Accepted alone.
var (
	isLoadTest     = os.Getenv("WARP_LOADTEST") == "1"
	ownTweetEvery  = envDuration("ECHO_TWEET_INTERVAL", ownTweetInterval)
	reactPercent   = envPercent("ECHO_REACT_PERCENT", 100)
	retweetPercent = envPercent("ECHO_RETWEET_PERCENT", 25)
	replyPercent   = envPercent("ECHO_REPLY_PERCENT", 25)
	followCount    = envCount("ECHO_FOLLOW_COUNT", 5)
	followDelay    = envDuration("ECHO_FOLLOW_DELAY", 90*time.Second)
	reactEvery     = envDuration("ECHO_REACT_INTERVAL", 30*time.Second)
	reactBatch     = envCount("ECHO_REACT_BATCH", 2000)
	timelinePage   = envCount("ECHO_TIMELINE_PAGE", 100)
)

func envCount(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 0 {
		log.Fatalf("%s must be a non-negative number, got %q", name, raw)
	}
	return count
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Fatalf("%s must be a positive duration, got %q", name, raw)
	}
	return d
}

func envPercent(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	percent, err := strconv.Atoi(raw)
	if err != nil || percent < 0 || percent > 100 {
		log.Fatalf("%s must be a number between 0 and 100, got %q", name, raw)
	}
	return percent
}

func rolled(percent int) bool { return percent > 0 && rand.IntN(100) < percent }

func echoIndexFromEnv() int {
	raw := os.Getenv("ECHO_INDEX")
	if raw == "" {
		return 0
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || index > 999 {
		log.Fatalf("ECHO_INDEX must be a number between 0 and 999, got %q", raw)
	}
	return index
}

// run node without GUI
func main() {
	version := config.Config().Version
	network := config.Config().Node.Network
	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		log.Fatal(err)
	}

	if config.Config().Logging.Format == config.TextFormat {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	} else {
		log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.DateTime})
	}
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	var interruptChan = make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		log.Errorf("failed to init db: %v \n", err)
		os.Exit(1)
		return
	}
	// Bring the in-memory DB up before seeding so the fixed Echo owner is
	// cached in AuthRepo; otherwise AuthLogin mints a fresh ULID each restart.
	if err := db.Run(username, echoPassword); err != nil {
		log.Fatalf("failed to run db: %v", err)
	}
	readyChan := make(chan domain.AuthNodeInfo, 10)

	authRepo := database.NewAuthRepo(db, network)
	authRepo.SetOwner(domain.Owner{
		CreatedAt:       time.Now(),
		UserId:          echoOwnerID,
		RedundantUserID: echoOwnerID,
		Username:        username,
	})

	userRepo := database.NewUserRepo(db)
	if _, err := userRepo.Create(domain.User{
		CreatedAt:     time.Now(),
		Id:            echoOwnerID,
		Username:      username,
		RoundTripTime: math.MaxInt64, // sit at the end of who-to-follow lists
	}); err != nil && !errors.Is(err, database.ErrUserAlreadyExists) {
		log.Fatalf("failed to pre-create echo user: %v", err)
	}

	authService := auth.NewAuthService(ctx, authRepo, userRepo, readyChan)

	go func() {
		_, authErr := authService.AuthLogin(event.LoginEvent{
			Username: username,
			Password: echoPassword,
		},
			psk,
		)
		if authErr != nil {
			log.Fatalf("failed to login: %v", authErr)
		}
	}()

	authInfo := <-readyChan

	nodeId, _ := warpnet.IDFromPublicKey(authRepo.PrivateKey().Public().(ed25519.PublicKey))

	bootstrapNodes, _ := config.Config().Node.AddrInfos()
	echoNode, err := member.NewMemberNode(
		ctx,
		authRepo.PrivateKey(),
		psk,
		nodeId,
		authRepo,
		db,
		bootstrapNodes,
	)
	if err != nil {
		log.Fatalf("failed to init node: %v", err)
	}
	defer echoNode.Stop()

	err = echoNode.Start()
	if err != nil {
		log.Fatalf("failed to start member node: %v", err)
	}

	authInfo.ID = nodeId.String()
	authInfo.Network = network
	authInfo.Addresses = echoNode.NodeInfo().Addresses
	authInfo.BootstrapPeers = config.Config().Node.Bootstrap

	readyChan <- authInfo
	echoFollowRepo := database.NewFollowRepo(db)
	echoTweetRepo := database.NewTweetRepo(db, nil)
	echoTimelineRepo := database.NewTimelineRepo(db)
	eBot := newEchoBot(
		echoNode, db, echoFollowRepo, echoTweetRepo,
		echoTimelineRepo, userRepo, authRepo.PrivateKey(),
	)
	go runOwnTweets(ctx, eBot, echoNode)
	if isLoadTest {
		go runReactions(ctx, eBot, echoNode)
	}
	setupHandlers(eBot, echoNode)

	log.Infoln("WARPNET STARTED")
	<-interruptChan
	log.Infoln("interrupted...")
}

type echoStreamClient interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	SelfStream(from, to warpnet.WarpPeerID, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type echoPersister interface {
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Get(key local_store.DatabaseKey) ([]byte, error)
}

type echoFollowStorer interface {
	Follow(fromUserId, toUserId string) error
}

type echoTweetStorer interface {
	Create(userId string, tweet domain.Tweet) (domain.Tweet, error)
}

type echoTimelineReader interface {
	GetTimeline(userId string, limit *uint64, cursor *string) ([]domain.Tweet, string, error)
}

type echoUserFetcher interface {
	Get(userId string) (domain.User, error)
}

const echoSeenNamespace = "/ECHO_SEEN/"

type echoBot struct {
	node         echoStreamClient
	db           echoPersister
	followRepo   echoFollowStorer
	tweetRepo    echoTweetStorer
	timelineRepo echoTimelineReader
	userRepo     echoUserFetcher
	privKey      ed25519.PrivateKey
	mu           sync.Mutex
	seen         map[string]time.Time
	lastPruneRun time.Time
}

func newEchoBot(
	node echoStreamClient, db echoPersister,
	followRepo echoFollowStorer, tweetRepo echoTweetStorer,
	timelineRepo echoTimelineReader, userRepo echoUserFetcher,
	privKey ed25519.PrivateKey,
) *echoBot {
	return &echoBot{
		node:         node,
		db:           db,
		followRepo:   followRepo,
		tweetRepo:    tweetRepo,
		timelineRepo: timelineRepo,
		userRepo:     userRepo,
		privKey:      privKey,
		seen:         make(map[string]time.Time),
		lastPruneRun: time.Now(),
	}
}

// Routes behind the auth middleware take a signed event.Message, not the bare
// payload — the same envelope the dashboard builds when it calls its own node.
// A fresh MessageId per call keeps the idempotency cache from replaying a reply.
func (e *echoBot) selfEnvelope(selfID warpnet.WarpPeerID, route stream.WarpRoute, body any) (event.Message, error) {
	bt, err := json.Marshal(body)
	if err != nil {
		return event.Message{}, err
	}
	msg := event.Message{
		Body:        bt,
		MessageId:   ulid.Make().String(),
		NodeId:      selfID.String(),
		Destination: string(route),
		Timestamp:   time.Now().UTC(),
		Version:     config.Config().Version.String(),
	}
	msg.Signature = security.Sign(e.privKey, msg.SigningBytes())
	return msg, nil
}

func (e *echoBot) wasSeen(action, id string) bool {
	if strings.TrimSpace(id) == "" {
		return false
	}
	key := action + ":" + id
	now := time.Now()

	e.mu.Lock()
	defer e.mu.Unlock()
	e.prune(now)
	if seenAt, ok := e.seen[key]; ok && now.Sub(seenAt) <= seenTTL {
		return true
	}

	// Check persistent storage for state surviving restarts
	if e.db != nil {
		dbKey := local_store.DatabaseKey(echoSeenNamespace + key)
		if _, err := e.db.Get(dbKey); err == nil {
			e.seen[key] = now
			return true
		}
	}

	e.seen[key] = now
	// Persist to database so state survives restarts
	if e.db != nil {
		dbKey := local_store.DatabaseKey(echoSeenNamespace + key)
		_ = e.db.SetWithTTL(dbKey, []byte(now.Format(time.RFC3339)), seenTTL)
	}
	e.evictIfNeeded()
	return false
}

func (e *echoBot) prune(now time.Time) {
	if now.Sub(e.lastPruneRun) < pruneInterval {
		return
	}
	expireBefore := now.Add(-seenTTL)
	for k, t := range e.seen {
		if t.Before(expireBefore) {
			delete(e.seen, k)
		}
	}
	e.lastPruneRun = now
}

func (e *echoBot) evictIfNeeded() {
	if len(e.seen) <= maxSeenKeys {
		return
	}
	var (
		oldestKey string
		oldestAt  time.Time
		set       bool
	)
	for k, t := range e.seen {
		if !set || t.Before(oldestAt) {
			oldestKey, oldestAt = k, t
			set = true
		}
	}
	if set {
		delete(e.seen, oldestKey)
	}
}

func (e *echoBot) ownerID() string {
	return e.node.NodeInfo().OwnerId
}

func (e *echoBot) handleFollow(msg []byte, requesterNodeID string) {
	var fl event.NewFollowEvent
	if err := json.Unmarshal(msg, &fl); err != nil {
		log.Warnf("echo: parse follow event: %v", err)
		return
	}
	if fl.FollowerId == "" || fl.FollowingId == "" {
		return
	}
	if fl.FollowerId == e.ownerID() || fl.FollowingId != e.ownerID() {
		return
	}
	if e.wasSeen("follow", fl.FollowerId) {
		return
	}
	if requesterNodeID == "" {
		return
	}

	if e.followRepo != nil {
		// X follows echo
		if err := e.followRepo.Follow(fl.FollowerId, e.ownerID()); err != nil &&
			!errors.Is(err, database.ErrAlreadyFollowed) {
			log.Warnf("echo: store inbound follow %s -> echo: %v", fl.FollowerId, err)
		}
		// echo follows X (auto-follow-back)
		if err := e.followRepo.Follow(e.ownerID(), fl.FollowerId); err != nil &&
			!errors.Is(err, database.ErrAlreadyFollowed) {
			log.Warnf("echo: store outbound follow echo -> %s: %v", fl.FollowerId, err)
		}
	}

	if _, err := e.node.GenericStream(
		requesterNodeID,
		event.PUBLIC_POST_FOLLOW,
		event.NewFollowEvent{FollowerId: e.ownerID(), FollowingId: fl.FollowerId},
	); err != nil {
		log.Warnf("echo: auto-follow-back failed: %v", err)
	}
}

func (e *echoBot) handleMessage(msg []byte, requesterNodeID string) {
	var m event.NewMessageEvent
	if err := json.Unmarshal(msg, &m); err != nil {
		log.Warnf("echo: parse message event: %v", err)
		return
	}
	if m.ChatId == "" || m.SenderId == "" || m.ReceiverId == "" {
		return
	}
	if m.SenderId == e.ownerID() || m.ReceiverId != e.ownerID() {
		return
	}
	if e.wasSeen("message", e.messageSeenKey(m)) {
		return
	}
	if requesterNodeID == "" {
		return
	}
	echoText := e.buildMessageReplyText(m.Text)
	quote, err := randomEchoText()
	if err == nil {
		echoText = quote
	}

	resp := event.NewMessageEvent{
		ChatId:     m.ChatId,
		SenderId:   e.ownerID(),
		ReceiverId: m.SenderId,
		Text:       echoText,
		CreatedAt:  time.Now(),
	}
	if _, err := e.node.GenericStream(requesterNodeID, event.PUBLIC_POST_MESSAGE, resp); err != nil {
		log.Warnf("echo: auto-chat-reply failed: %v", err)
	}
}

func (e *echoBot) messageSeenKey(m event.NewMessageEvent) string {
	if m.Id != "" {
		return m.Id
	}
	return fmt.Sprintf("%s|%s|%s|%d|%s", m.ChatId, m.SenderId, m.ReceiverId, m.CreatedAt.UnixNano(), m.Text)
}

func (e *echoBot) buildMessageReplyText(incomingText string) string {
	prefix := echoChatReply + ": "
	// Runes, not bytes: the chat handler counts runes, and cutting the text at
	// a byte offset would split an emoji and echo back U+FFFD.
	available := messageLimit - utf8.RuneCountInString(prefix)
	runes := []rune(incomingText)
	if len(runes) > available {
		return prefix + string(runes[:available])
	}
	return prefix + incomingText
}

// Reactions run off the timeline rather than off the delivery handler: the real
// PUBLIC_POST_TIMELINE handler owns the CRDT-backed tweet repo, and a bot that
// replaced it would silence the very counters the run is there to watch.
func (e *echoBot) reactToTimeline(selfID warpnet.WarpPeerID) {
	// GetTimeline sorts by time only *within* the page it fetched, and the page
	// itself comes back in key order — so a single unpaged call returns the same
	// oldest entries forever once the timeline outgrows it. Walk the cursor.
	var (
		tweets []domain.Tweet
		cursor *string
		page   = uint64(timelinePage)
	)
	for len(tweets) < reactBatch {
		batch, next, err := e.timelineRepo.GetTimeline(e.ownerID(), &page, cursor)
		if err != nil {
			log.Warnf("echo: read timeline: %v", err)
			break
		}
		tweets = append(tweets, batch...)
		if len(batch) == 0 || next == "" {
			break
		}
		cursor = &next
	}

	var fresh, roots, reacted, retweeted, replied, unresolved int
	for _, tw := range tweets {
		if tw.Id == "" || tw.UserId == e.ownerID() || e.wasSeen("reacted", tw.Id) {
			continue
		}
		fresh++
		if tw.ParentId == nil {
			roots++
		}

		// The author's node is what the reaction routes address, and it is only
		// known once their user record has been stored locally.
		author, err := e.userRepo.Get(tw.UserId)
		if err != nil || author.NodeId == "" {
			unresolved++
			continue
		}

		if rolled(reactPercent) {
			if err := e.reactToTweet(tw, author.NodeId); err != nil {
				log.Warnf("echo: react id=%s: %v", tw.Id, err)
			} else {
				reacted++
			}
		}
		if rolled(retweetPercent) {
			if err := e.retweet(tw, author.NodeId); err != nil {
				log.Warnf("echo: retweet id=%s: %v", tw.Id, err)
			} else {
				retweeted++
			}
		}
		// Only roots get replies: a reply federates as a tweet of its own, and
		// replying to replies would let one tweet amplify without bound.
		if tw.ParentId == nil && rolled(replyPercent) {
			if err := e.replyToTweet(tw, selfID); err != nil {
				log.Warnf("echo: reply id=%s: %v", tw.Id, err)
			} else {
				replied++
			}
		}
	}

	log.Infof("echo: timeline pass size=%d fresh=%d roots=%d reacted=%d retweeted=%d replied=%d unresolved=%d",
		len(tweets), fresh, roots, reacted, retweeted, replied, unresolved)
}

func runReactions(ctx context.Context, echo *echoBot, node *member.MemberNode) {
	ticker := time.NewTicker(reactEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			echo.reactToTimeline(node.NodeInfo().ID)
		}
	}
}

func (e *echoBot) reactToTweet(tw event.NewTweetEvent, requesterNodeID string) error {
	_, err := e.node.GenericStream(
		requesterNodeID,
		event.PUBLIC_POST_REACT,
		event.ReactionEvent{TweetId: tw.Id, UserId: tw.UserId, OwnerId: e.ownerID()},
	)
	return err
}

func (e *echoBot) retweet(tw event.NewTweetEvent, requesterNodeID string) error {
	retweeter := e.ownerID()
	_, err := e.node.GenericStream(
		requesterNodeID,
		event.PUBLIC_POST_RETWEET,
		event.NewRetweetEvent(domain.Tweet{
			Id:          tw.Id,
			RootId:      tw.RootId,
			ParentId:    tw.ParentId,
			Text:        tw.Text,
			UserId:      tw.UserId,
			Username:    tw.Username,
			CreatedAt:   tw.CreatedAt,
			RetweetedBy: &retweeter,
		}),
	)
	return err
}

// A reply is composed on our own node like any other tweet; ParentUserId is what
// makes it federate to the parent's author instead of staying local.
func (e *echoBot) replyToTweet(tw event.NewTweetEvent, selfID warpnet.WarpPeerID) error {
	parentID := tw.Id
	parentUserID := tw.UserId
	text := echoReplyPrefix + tw.Text
	if quote, err := randomEchoText(); err == nil {
		text = quote
	}
	rootID := tw.RootId
	if rootID == "" {
		rootID = tw.Id
	}
	reply := event.NewTweetEvent{
		CreatedAt:    time.Now(),
		Id:           ulid.Make().String(),
		ParentId:     &parentID,
		ParentUserId: &parentUserID,
		RootId:       rootID,
		Text:         text,
		UserId:       e.ownerID(),
		Username:     username,
	}
	envelope, err := e.selfEnvelope(selfID, event.PRIVATE_POST_TWEET, reply)
	if err != nil {
		return err
	}
	_, err = e.node.SelfStream(selfID, selfID, event.PRIVATE_POST_TWEET, envelope)
	return err
}

func setupHandlers(echo *echoBot, node *member.MemberNode) {
	// PRIVATE_POST_TWEET keeps its real handler: it is the compose route this
	// node calls on itself, and the auth middleware denies it to other peers
	// anyway, so there is nothing for the bot to answer there.
	node.Node().RemoveStreamHandler(event.PUBLIC_POST_MESSAGE)

	//nolint:govet
	handlers := []warpnet.WarpStreamHandler{
		{
			event.PUBLIC_POST_MESSAGE,
			func(msg []byte, s warpnet.WarpStream) (any, error) {
				echo.handleMessage(msg, requesterNodeID(s))
				return event.Accepted, nil
			},
		},
	}

	// Under load the real StreamFollowHandler has to stay: it is the only thing
	// that subscribes this node to a followee's gossip topic. The bot's
	// follow-back replaces it only when the node is answering people, not a run.
	if !isLoadTest {
		node.Node().RemoveStreamHandler(event.PUBLIC_POST_FOLLOW)
		//nolint:govet
		handlers = append(handlers, warpnet.WarpStreamHandler{
			event.PUBLIC_POST_FOLLOW,
			func(msg []byte, s warpnet.WarpStream) (any, error) {
				echo.handleFollow(msg, requesterNodeID(s))
				return event.Accepted, nil
			},
		})
	}

	node.SetStreamHandlers(handlers...)
}

func runOwnTweets(ctx context.Context, echo *echoBot, node *member.MemberNode) {
	if node == nil {
		log.Fatalf("echo: nil node")
	}

	if isLoadTest {
		// Give discovery time to fill the peerstore before asking who to follow.
		select {
		case <-ctx.Done():
			return
		case <-time.After(followDelay):
		}
		echo.followPeers(node.Node().Peerstore().PeersWithAddrs(), node.NodeInfo().ID, followCount)
	}

	ticker := time.NewTicker(ownTweetEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers := node.Node().Peerstore().PeersWithAddrs()
			echo.postOwnTweet(peers, node.NodeInfo().ID)
		}
	}
}

func (e *echoBot) postOwnTweet(peers []warpnet.WarpPeerID, selfID warpnet.WarpPeerID) string {
	text, err := randomEchoText()
	if err != nil || strings.TrimSpace(text) == "" {
		text = ownTweetFallback
	}
	if len(text) > ownTweetCharLimit {
		text = text[:ownTweetCharLimit]
	}

	tweetID := ulid.Make().String()
	tweet := event.NewTweetEvent{
		Id:        tweetID,
		RootId:    tweetID,
		UserId:    e.ownerID(),
		Username:  username,
		Text:      text,
		CreatedAt: time.Now(),
	}

	// Compose against our own node. PRIVATE_POST_TWEET is owner-only — the auth
	// middleware denies it to any other peer — and it is the compose route that
	// stores the tweet and hands it to the follower fan-out. GenericStream
	// refuses to dial ourselves, so this has to go through SelfStream.
	envelope, err := e.selfEnvelope(selfID, event.PRIVATE_POST_TWEET, tweet)
	if err != nil {
		log.Warnf("echo: own tweet id=%s envelope: %v", tweetID, err)
		return tweetID
	}
	if _, err := e.node.SelfStream(selfID, selfID, event.PRIVATE_POST_TWEET, envelope); err != nil {
		log.Warnf("echo: own tweet id=%s compose failed: %v", tweetID, err)
		return tweetID
	}
	log.Infof("echo: own tweet id=%s composed", tweetID)
	return tweetID
}

// A tweet only reaches other nodes through the author's followers, so the graph
// has to exist before any of this measures a fan-out. The peer's owner comes
// from its own node info; follow-back is what makes the pair mutual.
func (e *echoBot) followPeers(peers []warpnet.WarpPeerID, selfID warpnet.WarpPeerID, k int) int {
	var followed int
	var targets []string
	for _, peer := range peers {
		if followed >= k {
			break
		}
		if peer == selfID {
			continue
		}
		raw, err := e.node.GenericStream(peer.String(), event.PUBLIC_GET_INFO, nil)
		if err != nil {
			continue
		}
		var info warpnet.NodeInfo
		if err := json.Unmarshal(raw, &info); err != nil {
			continue
		}
		if info.OwnerId == "" || info.IsRelay() || info.OwnerId == e.ownerID() {
			continue
		}
		if e.wasSeen("following", info.OwnerId) {
			continue
		}
		// Against our own node: StreamFollowHandler is what subscribes us to the
		// followee's gossip topic, and without that subscription their fan-out
		// never reaches us. Storing the follow directly would skip it.
		envelope, err := e.selfEnvelope(selfID, event.PUBLIC_POST_FOLLOW,
			event.NewFollowEvent{FollowerId: e.ownerID(), FollowingId: info.OwnerId})
		if err != nil {
			log.Warnf("echo: follow %s envelope: %v", info.OwnerId, err)
			continue
		}
		if _, err := e.node.SelfStream(selfID, selfID, event.PUBLIC_POST_FOLLOW, envelope); err != nil {
			log.Warnf("echo: follow %s: %v", info.OwnerId, err)
			continue
		}
		followed++
		targets = append(targets, info.OwnerId)
	}
	log.Infof("echo: follow graph: self=%s following=%d of k=%d peers=%d targets=[%s]",
		e.ownerID(), followed, k, len(peers), strings.Join(targets, " "))
	return followed
}

func requesterNodeID(s warpnet.WarpStream) string {
	if s == nil || s.Conn() == nil {
		return ""
	}
	return s.Conn().RemotePeer().String()
}

// A group of echo nodes polling an external quote API would measure that API,
// not Warpnet. The suffix keeps every text distinct.
func randomEchoText() (string, error) {
	corpus := []string{
		"peers found is not peers reachable",
		"a counter that never converges is only a rumour",
		"gossip costs whatever the round-robin charges it",
		"every restart mints a generation that never leaves",
		"a dropped delta stays invisible until something counts it",
		"the limiter does not care which peer you needed",
		"back pressure is the message you never see",
		"convergence is a claim until two nodes agree on a number",
	}
	return fmt.Sprintf("%s #%d", corpus[rand.IntN(len(corpus))], rand.IntN(1_000_000)), nil
}
