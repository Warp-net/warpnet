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
	"crypto/ed25519"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	root "github.com/Warp-net/warpnet"
	bnode "github.com/Warp-net/warpnet/cmd/node/business/node"
	"github.com/Warp-net/warpnet/cmd/node/business/server/handlers"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	localstore "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/metrics"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

func main() {
	pw := config.Config().Node.Server.Password
	if pw == "" {
		log.Fatal("password is required")
	}
	network := config.Config().Node.Network
	version := config.Config().Version

	lvl, err := log.ParseLevel(config.Config().Logging.Level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
	if config.Config().Logging.Format == config.TextFormat {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	} else {
		log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.DateTime})
	}
	log.SetOutput(os.Stdout)

	log.Infof("network: %s", network)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	psk, err := security.GeneratePSK(network, version)
	if err != nil {
		log.Errorf("business: generate PSK: %v", err)
		return
	}
	codeHashHex, err := security.GetCodebaseHashHex(root.GetCodeBase())
	if err != nil {
		log.Errorf("business: codebase hash: %v", err)
		return
	}
	infos, err := config.Config().Node.AddrInfos()
	if err != nil {
		log.Errorf("business: bootstrap infos: %v", err)
		return
	}

	db, err := localstore.New(config.Config().Database.Path, localstore.DefaultOptions())
	if err != nil {
		log.Errorf("business: open db: %v", err)
		return
	}
	defer db.Close()

	readyChan := make(chan domain.AuthNodeInfo, 1)
	userRepo := database.NewUserRepo(db)
	authRepo := database.NewAuthRepo(db, network)
	authService := auth.NewAuthService(ctx, authRepo, userRepo, readyChan)

	staticHandler, err := handlers.NewStaticHandler()
	if err != nil {
		log.Fatalf("business: static handler load: %v", err)
	}

	bridgeHandler := handlers.NewBridgeHandler(
		security.AESCodec{Key: security.AESKeyFromPassword(pw)},
		authService,
		psk,
		db.IsFirstRun(),
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", bridgeHandler.Handle())
	mux.HandleFunc("/healthz", handlers.HealthHandler())
	mux.HandleFunc("/readyz", handlers.ReadyHandler())
	mux.Handle("/", staticHandler)

	srv := &http.Server{Addr: ":4999", Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	defer srv.Shutdown(ctx)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("business: serve http: %v", err)
		}
	}()

	log.Infof("business: listening on %s", srv.Addr)

	var info domain.AuthNodeInfo
	select {
	case <-ctx.Done():
		return
	case info = <-readyChan:
		log.Infoln("business: database authentication passed")
	}

	privateKey := authService.PrivateKey()
	ownNodeId, err := warpnet.IDFromPublicKey(privateKey.Public().(ed25519.PublicKey))
	if err != nil {
		log.Errorf("business: node ID: %v", err)
		return
	}

	m := metrics.NewMetricsClient(config.Config().Node.Metrics.Gateway, ownNodeId.String(), network)
	node, err := bnode.NewBusinessNode(
		ctx,
		privateKey,
		psk,
		ownNodeId,
		codeHashHex,
		version,
		authRepo,
		db,
		infos,
		m,
	)
	if err != nil {
		log.Errorf("business: init node: %v", err)
		return
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		log.Errorf("business: start node: %v", err)
		return
	}

	bridgeHandler.AttachNode(node)

	info.ID = ownNodeId.String()
	info.Network = network
	info.Addresses = node.NodeInfo().Addresses
	info.Role = node.NodeInfo().Type
	readyChan <- info

	<-interruptChan
	log.Infoln("business node interrupted...")
}
