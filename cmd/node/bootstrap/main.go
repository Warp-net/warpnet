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
	bootstrap "github.com/Warp-net/warpnet/cmd/node/bootstrap/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/metrics"
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
		log.Fatalf("bootstrap: fail generating key: %v", err)
	}
	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		log.Fatal(err)
	}

	n, err := bootstrap.NewBootstrapNode(ctx, privKey, psk, codeHashHex)
	if err != nil {
		log.Fatalf("failed to init bootstrap node: %v", err)
	}
	defer n.Stop()

	if err := n.Start(); err != nil {
		log.Fatalf("failed to start bootstrap node: %v", err)
	}

	m := metrics.NewMetricsClient(config.Config().Node.Metrics.Server, n.NodeInfo().ID.String(), true)
	m.PushStatusOnline()
	<-interruptChan
	log.Infoln("bootstrap node interrupted...")
}

// TODO temp. Check for https://github.com/libp2p/go-libp2p-kad-dht/issues/1073
func closeWriter() {
	defer func() { recover() }()
	_ = writer.WriterGroup.Close()
}
