//go:build backend

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
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Warp-net/warpnet/cmd/node/member/auth"
	member "github.com/Warp-net/warpnet/cmd/node/member/node"
	"github.com/Warp-net/warpnet/config"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/security"
	log "github.com/sirupsen/logrus"
)

// run node without GUI
func main() {
	psk, err := security.GeneratePSK("testnet", config.Config().Version)
	if err != nil {
		log.Fatal(err)
	}

	if config.Config().Logging.Format == config.TextFormat {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	} else {
		log.SetFormatter(&log.JSONFormatter{TimestampFormat: time.DateTime})
	}
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	var interruptChan = make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := local_store.New(config.Config().Database.Path, local_store.DefaultOptions())
	if err != nil {
		log.Errorf("failed to init db: %v \n", err)
		os.Exit(1)
		return
	}
	readyChan := make(chan domain.AuthNodeInfo, 10)

	authRepo := database.NewAuthRepo(db)
	userRepo := database.NewUserRepo(db)
	authService := auth.NewAuthService(ctx, authRepo, userRepo, readyChan)

	go func() {
		username, pass := manualCredsInput()

		_, err = authService.AuthLogin(event.LoginEvent{
			Username: username,
			Password: pass,
		})
		if err != nil {
			log.Fatalf("failed to login: %v", err)
		}
	}()

	authInfo := <-readyChan

	backendNode, err := member.NewMemberNode(
		ctx,
		authRepo.PrivateKey(),
		psk,
		"backend-only",
		config.Config().Version,
		authRepo,
		db,
	)
	if err != nil {
		log.Fatalf("failed to init node: %v", err)
	}
	defer backendNode.Stop()

	err = backendNode.Start()
	if err != nil {
		log.Fatalf("failed to start member node: %v", err)
	}

	authInfo.Identity.Owner.NodeId = backendNode.NodeInfo().ID.String()
	authInfo.NodeInfo = backendNode.NodeInfo()

	readyChan <- authInfo
	log.Infoln("WARPNET STARTED")
	<-interruptChan
	log.Infoln("interrupted...")
}

func manualCredsInput() (string, string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter username: ")
	username, _ := reader.ReadString('\n')
	fmt.Print("Enter password: ")
	pass, _ := reader.ReadString('\n')

	username = strings.TrimSuffix(username, "\n")
	username = strings.TrimSpace(username)
	pass = strings.TrimSuffix(pass, "\n")
	pass = strings.TrimSpace(pass)

	return username, pass

}
