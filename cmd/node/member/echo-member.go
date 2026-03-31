//go:build echo

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

package main

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/metrics"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
)

const (
	echoReplyPrefix = "echo: "
	echoChatReply   = "echo: получил сообщение"
)

// run node without GUI
func main() {
	psk, err := security.GeneratePSK(config.Config().Node.Network, config.Config().Version)
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

	db, err := local_store.New(config.Config().Database.Path, local_store.DefaultOptions())
	if err != nil {
		log.Errorf("failed to init db: %v \n", err)
		os.Exit(1)
		return
	}
	readyChan := make(chan domain.AuthNodeInfo, 10)

	authRepo := database.NewAuthRepo(db)
	userRepo := database.NewUserRepo(db)
	authService := auth.NewAuthService(ctx, authRepo, userRepo, readyChan)

	_, err = authService.AuthLogin(event.LoginEvent{
		Username: "Echo",
		Password: `\@4o97Z7<Cfu`,
	},
		psk,
	)
	if err != nil {
		log.Fatalf("failed to login: %v", err)
	}

	authInfo := <-readyChan

	m := metrics.NewMetricsClient(config.Config().Node.Metrics.Gateway, config.Config().Node.Network)

	echoNode, err := member.NewMemberNode(
		ctx,
		authRepo.PrivateKey(),
		psk,
		"echo",
		config.Config().Version,
		authRepo,
		db,
		m,
	)
	if err != nil {
		log.Fatalf("failed to init node: %v", err)
	}
	defer echoNode.Stop()

	err = echoNode.Start()
	if err != nil {
		log.Fatalf("failed to start member node: %v", err)
	}

	authInfo.Identity.Owner.NodeId = echoNode.NodeInfo().ID.String()
	authInfo.NodeInfo = echoNode.NodeInfo()

	readyChan <- authInfo
	setupHandlers(echoNode)
	log.Infoln("WARPNET STARTED")

	<-interruptChan
	log.Infoln("interrupted...")
}

type echoStreamClient interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type echoBot struct {
	node       echoStreamClient
	mu         sync.Mutex
	inProgress map[string]struct{}
}

func newEchoBot(node echoStreamClient) *echoBot {
	return &echoBot{node: node, inProgress: make(map[string]struct{})}
}

func (e *echoBot) seen(action, id string) bool {
	if id == "" {
		return false
	}
	key := action + ":" + id
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.inProgress[key]; ok {
		return true
	}
	e.inProgress[key] = struct{}{}
	return false
}

func (e *echoBot) ownerID() string {
	return e.node.NodeInfo().OwnerId
}

func (e *echoBot) handleTweet(msg []byte) {
	var tw event.NewTweetEvent
	if err := json.Unmarshal(msg, &tw); err != nil {
		log.Warnf("echo: parse tweet event: %v", err)
		return
	}
	if tw.UserId == "" || tw.Id == "" {
		return
	}
	if tw.UserId == e.ownerID() {
		return
	}
	if e.seen("tweet", tw.Id) {
		return
	}

	if err := e.likeTweet(tw); err != nil {
		log.Warnf("echo: auto-like failed: %v", err)
	}
	if err := e.retweet(tw); err != nil {
		log.Warnf("echo: auto-retweet failed: %v", err)
	}
	if err := e.replyToTweet(tw); err != nil {
		log.Warnf("echo: auto-reply-tweet failed: %v", err)
	}
}

func (e *echoBot) handleReply(msg []byte) {
	var rp event.NewReplyEvent
	if err := json.Unmarshal(msg, &rp); err != nil {
		log.Warnf("echo: parse reply event: %v", err)
		return
	}
	if rp.UserId == "" || rp.Id == "" || rp.RootId == "" {
		return
	}
	if rp.UserId == e.ownerID() {
		return
	}
	if e.seen("reply", rp.Id) {
		return
	}

	if err := e.replyToReply(rp); err != nil {
		log.Warnf("echo: auto-reply-reply failed: %v", err)
	}
}

func (e *echoBot) handleFollow(msg []byte) {
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
	if e.seen("follow", fl.FollowerId) {
		return
	}

	if _, err := e.node.GenericStream(
		e.node.NodeInfo().ID.String(),
		event.PUBLIC_POST_FOLLOW,
		event.NewFollowEvent{FollowerId: e.ownerID(), FollowingId: fl.FollowerId},
	); err != nil {
		log.Warnf("echo: auto-follow-back failed: %v", err)
	}
}

func (e *echoBot) handleMessage(msg []byte) {
	var m event.NewMessageEvent
	if err := json.Unmarshal(msg, &m); err != nil {
		log.Warnf("echo: parse message event: %v", err)
		return
	}
	if m.Id == "" || m.ChatId == "" || m.SenderId == "" || m.ReceiverId == "" {
		return
	}
	if m.SenderId == e.ownerID() || m.ReceiverId != e.ownerID() {
		return
	}
	if e.seen("message", m.Id) {
		return
	}

	resp := event.NewMessageEvent{
		ChatId:     m.ChatId,
		SenderId:   e.ownerID(),
		ReceiverId: m.SenderId,
		Text:       fmt.Sprintf("%s: %s", echoChatReply, m.Text),
		CreatedAt:  time.Now(),
	}
	if _, err := e.node.GenericStream(e.node.NodeInfo().ID.String(), event.PUBLIC_POST_MESSAGE, resp); err != nil {
		log.Warnf("echo: auto-chat-reply failed: %v", err)
	}
}

func (e *echoBot) likeTweet(tw event.NewTweetEvent) error {
	_, err := e.node.GenericStream(
		e.node.NodeInfo().ID.String(),
		event.PUBLIC_POST_LIKE,
		event.LikeEvent{TweetId: tw.Id, UserId: tw.UserId, OwnerId: e.ownerID()},
	)
	return err
}

func (e *echoBot) retweet(tw event.NewTweetEvent) error {
	retweeter := e.ownerID()
	_, err := e.node.GenericStream(
		e.node.NodeInfo().ID.String(),
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

func (e *echoBot) replyToTweet(tw event.NewTweetEvent) error {
	parentID := tw.Id
	_, err := e.node.GenericStream(
		e.node.NodeInfo().ID.String(),
		event.PUBLIC_POST_REPLY,
		event.NewReplyEvent{
			CreatedAt:    time.Now(),
			Id:           ulid.Make().String(),
			ParentId:     &parentID,
			ParentUserId: tw.UserId,
			RootId:       tw.Id,
			Text:         echoReplyPrefix + tw.Text,
			UserId:       e.ownerID(),
			Username:     "Echo",
		},
	)
	return err
}

func (e *echoBot) replyToReply(rp event.NewReplyEvent) error {
	parentID := rp.Id
	_, err := e.node.GenericStream(
		e.node.NodeInfo().ID.String(),
		event.PUBLIC_POST_REPLY,
		event.NewReplyEvent{
			CreatedAt:    time.Now(),
			Id:           ulid.Make().String(),
			ParentId:     &parentID,
			ParentUserId: rp.UserId,
			RootId:       rp.RootId,
			Text:         echoReplyPrefix + rp.Text,
			UserId:       e.ownerID(),
			Username:     "Echo",
		},
	)
	return err
}

func setupHandlers(node *member.MemberNode) {
	echo := newEchoBot(node)

	//nolint:govet
	node.SetStreamHandlers(
		[]warpnet.WarpStreamHandler{
			{
				event.PRIVATE_POST_TWEET,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					echo.handleTweet(msg)
					return event.Accepted, nil
				}},
			{
				event.PUBLIC_POST_REPLY,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					echo.handleReply(msg)
					return event.Accepted, nil
				}},
			{
				event.PUBLIC_POST_FOLLOW,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					echo.handleFollow(msg)
					return event.Accepted, nil
				}},
			{
				event.PUBLIC_POST_LIKE,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					return event.Accepted, nil
				}},
			{
				event.PUBLIC_POST_RETWEET,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					return event.Accepted, nil
				}},
			{
				event.PUBLIC_POST_MESSAGE,
				func(msg []byte, s warpnet.WarpStream) (any, error) {
					echo.handleMessage(msg)
					return event.Accepted, nil
				},
			},
		}...,
	)
}
