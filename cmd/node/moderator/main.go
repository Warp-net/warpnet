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
	"os"
	"os/signal"
	"syscall"
	"time"

	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/cmd/node/moderator/moderator"
	"github.com/Warp-net/warpnet/cmd/node/moderator/node"
	"github.com/Warp-net/warpnet/cmd/node/moderator/pubsub"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

func main() {
	if config.Config().Node.Moderator.Path == "" {
		log.Errorln("moderator not configured: model path is empty")
		return
	}

	version := config.Config().Version
	network := config.Config().Node.Network
	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		log.Errorf("moderator: fail generating PSK: %v", err)
		return
	}

	lvl, err := log.ParseLevel(config.Config().Logging.Level)
	if err != nil {
		log.Errorf(
			"failed to parse log level %s: %v, defaulting to ERROR level...",
			config.Config().Logging.Level, err,
		)
		lvl = log.ErrorLevel
	}
	log.SetLevel(lvl)
	if config.Config().Logging.Format == config.TextFormat {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	} else {
		log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.DateTime})
	}
	log.SetOutput(os.Stdout) // stderr reserved for llama

	var interruptChan = make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := []byte(config.Config().Node.Seed)
	privKey, err := security.GenerateKeyFromSeed(seed)
	if err != nil {
		log.Errorf("moderator: fail generating key: %v", err)
		return
	}
	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		log.Errorln(err)
		return
	}

	n, err := node.NewModeratorNode(ctx, privKey, psk, codeHashHex)
	if err != nil {
		log.Errorf("failed to init moderator node: %v", err)
		return
	}

	if err = n.Start(); err != nil {
		log.Errorf("failed to start moderator node: %v", err)
		return
	}
	defer n.Stop()

	publisher := pubsub.NewPubSub(ctx)
	if err := publisher.Run(n); err != nil {
		log.Errorf("failed to start moderator pubsub: %v", err)
		return
	}
	defer func() {
		_ = publisher.Close()
	}()

	moder, err := moderator.NewModerator(ctx, n, publisher)
	if err != nil {
		log.Errorf("failed to init moderator: %v", err)
		return
	}
	if err := moder.Start(); err != nil {
		log.Errorf("failed to start moderator: %v", err)
		return
	}
	defer moder.Close()

	<-interruptChan
	log.Infoln("moderator node interrupted...")
}
