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
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/cmd/node/moderator/moderator"
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
	log.SetOutput(os.Stdout) // stderr reserved for llama

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
	authSvc := auth.NewAuthService(ctx, database.NewAuthRepo(db, network), database.NewUserRepo(db), readyChan)

	var wsKey []byte
	if pw := config.Config().Node.Server.Password; pw != "" {
		wsKey = security.AESKeyFromPassword(pw)
	} else {
		log.Warnln("business: node.server.password is empty — dashboard WS traffic is NOT encrypted")
	}

	srv, err := server.New(authSvc, psk, wsKey, db)
	if err != nil {
		log.Errorf("business: init server: %v", err)
		return
	}

	// The node (and its moderator) live separately from the dashboard. They are
	// started here on the first login and attached to the server.
	go runNode(ctx, nodeDeps{
		auth:        authSvc,
		psk:         psk,
		codeHashHex: codeHashHex,
		infos:       infos,
		db:          db,
		readyChan:   readyChan,
	}, srv)

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

type nodeDeps struct {
	auth        *auth.AuthService
	psk         security.PSK
	codeHashHex string
	infos       []warpnet.WarpAddrInfo
	db          *localstore.DB
	readyChan   chan domain.AuthNodeInfo
}

// runNode owns the node lifecycle, independent of the dashboard: it waits for
// the first login, builds and starts the node, stamps the owner's role, starts
// the reachability tracker, wires a separate moderator, hands the node to the
// dashboard, and tears everything down on shutdown.
func runNode(ctx context.Context, d nodeDeps, srv *server.Server) {
	var info domain.AuthNodeInfo
	select {
	case <-ctx.Done():
		return
	case info = <-d.readyChan:
		log.Infoln("business: database authentication passed")
	}

	ownNodeId, err := warpnet.IDFromPublicKey(d.auth.PrivateKey().Public().(ed25519.PublicKey))
	if err != nil {
		log.Errorf("business: node ID: %v", err)
		return
	}

	m := metrics.NewMetricsClient(config.Config().Node.Metrics.Gateway, ownNodeId.String(), config.Config().Node.Network)
	bn, err := bnode.NewBusinessNode(
		ctx, d.auth.PrivateKey(), d.psk, ownNodeId, d.codeHashHex,
		config.Config().Version, d.auth.Storage(), d.db, d.infos, m,
	)
	if err != nil {
		log.Errorf("business: init node: %v", err)
		return
	}
	if err := bn.Start(); err != nil {
		log.Errorf("business: start node: %v", err)
		return
	}

	// Stamp the role onto the owner's record so it travels to peers via
	// PUBLIC_GET_USER + discovery. Any node could do the same with its own role.
	if err := database.NewUserRepo(d.db).SetRole(info.UserId, warpnet.BusinessRole); err != nil {
		log.Warnf("business: set owner role: %v", err)
	}

	go bn.TrackPublicReachability(ctx)

	moder := startModerator(ctx, bn)

	srv.Attach(bn)

	info.ID = ownNodeId.String()
	info.Network = config.Config().Node.Network
	info.Addresses = bn.NodeInfo().Addresses
	d.readyChan <- info

	<-ctx.Done()
	if moder != nil {
		moder.Close()
	}
	bn.Stop()
}

// startModerator wires the moderator engine to the node's gossip as a separate
// entity. No-op (nil) when no model path is configured. moderator.Start blocks
// until the engine is ready (or forever without the `llama` tag), so it runs on
// its own goroutine.
func startModerator(ctx context.Context, bn *bnode.BusinessNode) *moderator.Moderator {
	if config.Config().Node.Moderator.Path == "" {
		log.Warnln("business: moderator model path is empty; moderation disabled")
		return nil
	}
	g := bn.Gossip()
	if g == nil {
		log.Errorln("business: moderator: gossip not running")
		return nil
	}
	moder, err := moderator.NewModerator(ctx, bn, g, g)
	if err != nil {
		log.Errorf("business: init moderator: %v", err)
		return nil
	}
	go func() {
		log.Infoln("business: starting moderator engine...")
		if err := moder.Start(); err != nil {
			log.Errorf("business: moderator start: %v", err)
		}
	}()
	return moder
}
