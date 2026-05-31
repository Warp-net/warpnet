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
	"github.com/Warp-net/warpnet/cmd/node/business/server"
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
	authSvc := auth.NewAuthService(ctx, database.NewAuthRepo(db, network), userRepo, readyChan)

	var wsKey []byte
	if pw := config.Config().Node.Server.Password; pw != "" {
		wsKey = security.AESKeyFromPassword(pw)
	} else {
		log.Warnln("business: node.server.password is empty — dashboard WS traffic is NOT encrypted")
	}

	disp := handlers.NewDispatcher(authSvc, db, psk)
	srv, err := server.New(disp, wsKey)
	if err != nil {
		log.Errorf("business: init server: %v", err)
		return
	}

	// The node is started here, separately from the dashboard, on the first
	// login, and attached to the dispatcher.
	go func() {
		var info domain.AuthNodeInfo
		select {
		case <-ctx.Done():
			return
		case info = <-readyChan:
			log.Infoln("business: database authentication passed")
		}

		ownNodeId, err := warpnet.IDFromPublicKey(authSvc.PrivateKey().Public().(ed25519.PublicKey))
		if err != nil {
			log.Errorf("business: node ID: %v", err)
			return
		}
		m := metrics.NewMetricsClient(config.Config().Node.Metrics.Gateway, ownNodeId.String(), network)

		node, err := bnode.NewBusinessNode(ctx, authSvc.PrivateKey(), psk, ownNodeId, codeHashHex, version, authSvc.Storage(), db, infos, m)
		if err != nil {
			log.Errorf("business: init node: %v", err)
			return
		}
		if err := node.Start(); err != nil {
			log.Errorf("business: start node: %v", err)
			return
		}
		defer node.Stop()

		// Stamp the role onto the owner's record so it travels to peers via
		// PUBLIC_GET_USER + discovery. Any node could do the same with its role.
		if err := userRepo.SetRole(info.UserId, warpnet.BusinessRole); err != nil {
			log.Warnf("business: set owner role: %v", err)
		}
		go node.TrackPublicReachability(ctx)

		disp.Attach(node)
		info.ID = ownNodeId.String()
		info.Network = network
		info.Addresses = node.NodeInfo().Addresses
		readyChan <- info

		<-ctx.Done()
	}()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Run(":" + config.Config().Node.Server.Port) }()

	select {
	case <-interruptChan:
		log.Infoln("business node interrupted...")
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("business: serve: %v", err)
		}
	}
	cancel()
	_ = srv.Shutdown()
}
