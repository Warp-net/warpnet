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
	root "github.com/Warp-net/warpnet"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/node/moderator"
	"github.com/Warp-net/warpnet/security"
	writer "github.com/ipfs/go-log/writer"
	log "github.com/sirupsen/logrus"
	_ "go.uber.org/automaxprocs" // DO NOT remove
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	defer closeWriter()
	if config.Config().Node.Moderator.Path == "" && config.Config().Node.Moderator.CID == "" {
		log.Errorln("moderator IPFS node not configured: model path and CID are empty")
		return
	}

	version := config.Config().Version
	network := config.Config().Node.Network
	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		panic(err)
	}

	lvl, err := log.ParseLevel(config.Config().Logging.Level)
	if err != nil {
		log.Errorf(
			"failed to parse log level %s: %v, defaulting to INFO level...",
			config.Config().Logging.Level, err,
		)
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.DateTime,
		FieldMap: log.FieldMap{
			"network": config.Config().Node.Network,
		},
	})

	var interruptChan = make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seed := []byte(config.Config().Node.Seed)
	privKey, err := security.GenerateKeyFromSeed(seed)
	if err != nil {
		log.Fatalf("moderator: fail generating key: %v", err)
	}
	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		log.Fatal(err)
	}

	n, err := moderator.NewModeratorNode(ctx, privKey, psk, codeHashHex)
	if err != nil {
		log.Fatalf("failed to init moderator node: %v", err)
	}
	defer n.Stop()

	if err := n.Start(); err != nil {
		log.Errorf("failed to start moderator node: %v", err)
		return
	}

	<-interruptChan
	log.Infoln("moderator node interrupted...")
}

// TODO temp. Check for https://github.com/libp2p/go-libp2p-kad-dht/issues/1073
func closeWriter() {
	defer func() { recover() }()
	_ = writer.WriterGroup.Close()
}
