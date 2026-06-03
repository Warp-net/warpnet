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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command fediverse-gateway is a thin ActivityPub gateway that lets a single
// Warpnet user be discovered and followed from Mastodon / the Fediverse.
//
// This is the Phase-1 skeleton: it serves WebFinger, an actor document with
// an RSA public key, and an inbox that verifies HTTP signatures and answers
// inbound Follow activities with a signed Accept. It does not yet publish
// posts outbound or translate inbound interactions into Warpnet — those are
// Phase 2/3 (see README.md).
//
// The gateway holds no user content. It is meant to run behind a tunnel that
// terminates TLS (Tailscale Funnel, Cloudflare Tunnel, …) so it never deals
// with certificates itself; -host is the public hostname that tunnel exposes.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

const gatewayVersion = "0.1.0"

func main() {
	var (
		host    = flag.String("host", envOr("GATEWAY_HOST", ""), "public hostname the tunnel exposes, e.g. name.tailnet.ts.net (no scheme)")
		addr    = flag.String("addr", envOr("GATEWAY_ADDR", "127.0.0.1:8080"), "local listen address the tunnel forwards to")
		keyPath = flag.String("key", envOr("GATEWAY_KEY", "fediverse-gateway-key.pem"), "path to the RSA private key (created on first run)")
		user    = flag.String("user", envOr("GATEWAY_USER", "warpnet"), "preferredUsername of the bridged actor (the part before @host)")
		display = flag.String("display-name", envOr("GATEWAY_DISPLAY_NAME", "Warpnet"), "display name shown on the actor")
		summary = flag.String("summary", envOr("GATEWAY_SUMMARY", "Warpnet ↔ Fediverse gateway (skeleton)"), "actor bio/summary")
	)
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	if *host == "" {
		log.Fatalln("gateway: -host is required (the public hostname your tunnel exposes)")
	}

	key, err := loadOrCreateKey(*keyPath)
	if err != nil {
		log.Fatalf("gateway: %v", err)
	}
	pubPEM, err := publicKeyPEM(key)
	if err != nil {
		log.Fatalf("gateway: %v", err)
	}

	wu := warpnetUser{
		ID:                *user,
		PreferredUsername: *user,
		DisplayName:       *display,
		Summary:           *summary,
	}

	g := &gateway{
		host:        *host,
		key:         key,
		keyPubPEM:   pubPEM,
		source:      staticSource{user: wu},
		signingUser: *user,
		client:      &http.Client{Timeout: 15 * time.Second},
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           g.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Infof("gateway: listening on %s, public https://%s", *addr, *host)
		log.Infof("gateway: bridged actor is @%s@%s -> %s", *user, *host, g.actorID(*user))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Infoln("gateway: shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
