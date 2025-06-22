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

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	log "github.com/sirupsen/logrus"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TODO consensus validation
func NewModeratorPubSub(ctx context.Context) *warpModeratorPubSub {
	return &warpModeratorPubSub{
		ctx:        ctx,
		pubsub:     nil,
		serverNode: nil,
		mx:         new(sync.RWMutex),
		sub:        nil,
		topic:      nil,
		isRunning:  new(atomic.Bool),
	}
}

func (g *warpModeratorPubSub) Run(
	serverNode PubsubModeratorConnector,
) {
	if g.isRunning.Load() {
		return
	}

	g.serverNode = serverNode

	if err := g.runPubSub(serverNode); err != nil {
		log.Errorf("pubsub moderator: failed to run: %v", err)
		return
	}

	go func() {
		if err := g.runListener(); err != nil {
			log.Errorf("pubsub moderator: listener: %v", err)
			return
		}
		log.Infoln("pubsub moderator: listener stopped")
	}()
}

func (g *warpModeratorPubSub) runListener() error {
	if g == nil {
		return warpnet.WarpError("pubsub moderator: service not initialized properly")
	}
	for {
		if !g.isRunning.Load() {
			return nil
		}

		if err := g.ctx.Err(); err != nil {
			return err
		}

		g.mx.RLock()
		sub := g.sub
		g.mx.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)

		msg, err := sub.Next(ctx)
		cancel()
		if errors.Is(err, pubsub.ErrSubscriptionCancelled) {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			continue
		}
		if err != nil {
			log.Errorf("pubsub moderator: failed to listen subscription to topic: %v", err)
			continue
		}
		if msg.Topic == nil {
			continue
		}

		switch strings.TrimSpace(*msg.Topic) {
		case pubSubModerationTopic:
			// TODO
		default:
			continue
		}
	}
}

func (g *warpModeratorPubSub) runPubSub(n PubsubModeratorConnector) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pubsub moderator: recovered from panic: %v", r)
		}
	}()
	if g == nil {
		return warpnet.WarpError("pubsub moderator: service not initialized properly")
	}

	g.pubsub, err = pubsub.NewGossipSub(g.ctx, n.Node())
	if err != nil {
		return err
	}
	g.isRunning.Store(true)

	if err := g.subscribe(pubSubModerationTopic); err != nil {
		return err
	}

	log.Infoln("pubsub moderator: started")

	return nil
}

func (g *warpModeratorPubSub) subscribe(topicName string) (err error) {
	if g == nil || !g.isRunning.Load() {
		return warpnet.WarpError("pubsub moderator: service not initialized")
	}
	g.mx.Lock()
	defer g.mx.Unlock()

	if topicName == "" {
		return warpnet.WarpError("pubsub moderator: topic name is empty")
	}

	if g.topic == nil {
		g.topic, err = g.pubsub.Join(topicName)
		if err != nil {
			return err
		}
	}

	sub, err := g.topic.Subscribe()
	if err != nil {
		return err
	}

	log.Infof("pubsub moderator: subscribed to topic: %s", topicName)

	g.sub = sub

	return nil
}

func (g *warpModeratorPubSub) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	if !g.isRunning.Load() {
		return
	}

	g.mx.Lock()
	defer g.mx.Unlock()

	g.sub.Cancel()

	_ = g.topic.Close()

	g.isRunning.Store(false)

	g.pubsub = nil
	g.topic = nil
	g.sub = nil
	log.Infoln("pubsub moderator: closed")
	return
}
