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
	"github.com/Warp-net/warpnet/cmd/node/business/server"
	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/database"
	localstore "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
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

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-interruptChan
		log.Infoln("business node interrupted...")
		cancel()
	}()

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
	authRepo := database.NewAuthRepo(db, network)
	userRepo := database.NewUserRepo(db)
	readyChan := make(chan domain.AuthNodeInfo, 1)

	var wsKey []byte
	if pw := config.Config().Node.Server.Password; pw != "" {
		wsKey = security.AESKeyFromPassword(pw)
	} else {
		log.Warnln("business: node.server.password is empty — dashboard WS traffic is NOT encrypted")
	}

	srv := server.New(ctx, server.Deps{
		DB:             db,
		Auth:           auth.NewAuthService(ctx, authRepo, userRepo, readyChan),
		PSK:            psk,
		CodeHashHex:    codeHashHex,
		Bootstrap:      infos,
		Network:        network,
		Version:        version,
		MetricsGateway: config.Config().Node.Metrics.Gateway,
		ReadyChan:      readyChan,
		WSKey:          wsKey,
	})
	defer srv.Close()

	if err := srv.Run(":" + config.Config().Node.Server.Port); err != nil {
		log.Errorf("business: serve: %v", err)
	}
}
