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
	"errors"
	"fmt"
	handlers2 "github.com/Warp-net/warpnet/cmd/node/business/handlers"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

func main() {
	pw := config.Config().Node.Server.Password
	if pw == "" {
		log.Fatal("password is required")
	}
	port := config.Config().Node.Server.Port
	version := config.Config().Version
	dbDir := config.Config().Database.Dir

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	staticHandler, err := handlers2.NewStaticHandler()
	if err != nil {
		log.Errorf("business: static handler load: %v", err)
		return
	}

	readyChan := make(chan domain.AuthNodeInfo, 1)

	bridgeHandler := handlers2.NewBridgeHandler(
		ctx,
		security.AESCodec{Key: security.AESKeyFromPassword(pw)},
		dbDir,
		version,
		readyChan,
	)

	mux := http.NewServeMux()
	mux.Handle("/ws", bridgeHandler.Handle())
	mux.HandleFunc("/healthz", handlers2.HealthHandler())
	mux.HandleFunc("/readyz", handlers2.ReadyHandler())
	mux.Handle("/", staticHandler)

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	defer srv.Shutdown(ctx) //nolint:errcheck
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("business: serve http: %v", err)
		}
	}()

	fmt.Printf("\033[1mNODE IS LISTENING ON 'localhost:%s'. PUT THIS ADDRESS INTO A BROWSER \033[0m\n", srv.Addr)

	// Start the node once login authenticates the network-scoped database.
	go bridgeHandler.RunNode()

	<-interruptChan
	log.Infoln("business node interrupted...")
}
