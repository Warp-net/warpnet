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

	"github.com/Warp-net/warpnet/config"
	log "github.com/sirupsen/logrus"
)

func main() {
	if !config.Config().Node.Business.Enabled {
		log.Errorln("business node not enabled: pass --node.business.enabled")
		return
	}

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

	log.Infof("network: %s", config.Config().Node.Network)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-interruptChan
		log.Infoln("business node interrupted...")
		cancel()
	}()

	app := NewApp()
	if err := app.Startup(ctx); err != nil {
		log.Errorf("business: startup failed: %v", err)
		return
	}
	defer app.Close()

	addr := ":" + config.Config().Node.Business.HttpPort
	if err := app.Serve(addr); err != nil {
		log.Errorf("business: serve: %v", err)
	}
}
