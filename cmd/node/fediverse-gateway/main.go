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
// Warpnet user be discovered and followed from Mastodon / the Fediverse and
// federates that user's posts outbound to their Fediverse followers.
//
// Implemented: WebFinger, an actor document with an RSA public key, an inbox
// that verifies HTTP signatures and answers Follow with a signed Accept
// (persisting the follower), outbound Create(Note) fan-out, and a libp2p
// connector to the Warpnet network (GATEWAY_PROBE smoke-tests it against
// GATEWAY_NODE_ADDR).
//
// Configuration is environment-only (GATEWAY_* below, plus the standard
// NODE_NETWORK for the libp2p side). It does NOT use CLI flags: importing the
// libp2p stack pulls in config.init's pflag.Parse, which would clash with a
// second flag set, and every other Warpnet node is env-configured too.
//
// The gateway keeps only keys on disk (RSA signing key); profile/followers
// live in Warpnet. It is meant to run behind a tunnel that terminates TLS
// (Tailscale Funnel, Cloudflare Tunnel, …); GATEWAY_HOST is the public
// hostname that tunnel exposes.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Warp-net/warpnet/core/warpnet"
	log "github.com/sirupsen/logrus"
	"tailscale.com/tsnet"
)

const gatewayVersion = "0.1.0"

const fatalFmt = "gateway: %v"

// defaultNetwork is the Warpnet network the connector joins unless NODE_NETWORK
// overrides it.
const defaultNetwork = "warpnet"

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.DateTime})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	// Smoke-test the libp2p connector against the configured Warpnet node.
	if envOr("GATEWAY_PROBE", "") != "" {
		runProbe()
		return
	}

	var (
		host          = envOr("GATEWAY_HOST", "")
		addr          = envOr("GATEWAY_ADDR", "127.0.0.1:8080")
		keyPath       = envOr("GATEWAY_KEY", "fediverse-gateway-key.pem")
		user          = envOr("GATEWAY_USER", "warpnet")
		display       = envOr("GATEWAY_DISPLAY_NAME", "Warpnet")
		summary       = envOr("GATEWAY_SUMMARY", "Warpnet ↔ Fediverse gateway (skeleton)")
		followersPath = envOr("GATEWAY_FOLLOWERS", "fediverse-gateway-followers.json")
	)

	// Optionally bring up the public endpoint via embedded Tailscale Funnel: the
	// gateway becomes its own tailnet node and ListenFunnel (below) serves public
	// HTTPS with an auto-provisioned *.ts.net cert, so host is derived from the
	// node. The persisted Dir keeps the hostname stable across restarts (a
	// rotating name orphans existing followers). Without the flag tsnet is never
	// instantiated.
	var ts *tsnet.Server
	if envOr("GATEWAY_FUNNEL", "") != "" {
		ts = &tsnet.Server{
			Hostname: envOr("GATEWAY_FUNNEL_HOSTNAME", "warpnet-gw"),
			Dir:      envOr("GATEWAY_FUNNEL_DIR", "fediverse-gateway-tsnet"),
			AuthKey:  os.Getenv("TS_AUTHKEY"),
			UserLogf: log.Infof,
			Logf:     log.Debugf,
		}
		st, uerr := ts.Up(context.Background())
		if uerr != nil {
			log.Fatalf("gateway: tailscale funnel: %v", uerr)
		}
		if st.Self == nil || st.Self.DNSName == "" {
			log.Fatalln("gateway: tailscale funnel: node has no DNS name (enable MagicDNS + HTTPS for the tailnet)")
		}
		host = strings.TrimSuffix(st.Self.DNSName, ".")
		log.Infof("gateway: tailscale funnel node up as %s", host)
	}

	if host == "" {
		log.Fatalln("gateway: GATEWAY_HOST is required (the public hostname your tunnel exposes), or set GATEWAY_FUNNEL=1")
	}

	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf(fatalFmt, err)
	}
	pubPEM, err := publicKeyPEM(key)
	if err != nil {
		log.Fatalf(fatalFmt, err)
	}

	wu := warpnetUser{
		ID:                user,
		PreferredUsername: user,
		DisplayName:       display,
		Summary:           summary,
	}

	// Profile source: read live from a Warpnet node when GATEWAY_NODE_ADDR is
	// set (state lives in Warpnet), otherwise a static operator-configured stub.
	appCtx, appCancel := context.WithCancel(context.Background())

	var src warpnetSource = staticSource{user: wu}
	var nodeCli *nodeClient
	if nodeAddr := envOr("GATEWAY_NODE_ADDR", ""); nodeAddr != "" {
		target, terr := warpnet.AddrInfoFromString(nodeAddr)
		if terr != nil {
			appCancel()
			log.Fatalf("gateway: bad GATEWAY_NODE_ADDR: %v", terr)
		}
		nodeCli, err = newNodeClient(appCtx, envOr("NODE_NETWORK", defaultNetwork), nil, *target)
		if err != nil {
			appCancel()
			log.Fatalf("gateway: connect Warpnet node: %v", err)
		}
		src = nodeSource{client: nodeCli, userID: user}
		log.Infof("gateway: profile sourced from Warpnet node %s", target.ID)
	}

	// Follower graph lives in Warpnet (via the node connector); only when no
	// node is configured does the gateway fall back to a local dev store.
	var followers followerStore
	var req nodeRequester
	if nodeCli != nil {
		followers = nodeFollowerStore{req: nodeCli}
		req = nodeCli
	} else {
		ff, ferr := newFileFollowerStore(followersPath)
		if ferr != nil {
			appCancel()
			log.Fatalf(fatalFmt, ferr)
		}
		followers = ff
	}

	g := &gateway{
		host:        host,
		key:         key,
		keyPubPEM:   pubPEM,
		source:      src,
		signingUser: user,
		client:      &http.Client{Timeout: 15 * time.Second},
		sem:         make(chan struct{}, maxInflightDeliveries),
		followers:   followers,
		req:         req,
	}

	// Federate the owner's new tweets to Fediverse followers.
	if nodeCli != nil {
		go newTweetPoller(nodeCli, user, g.publishNote).run(appCtx)
	}

	srv := &http.Server{
		Handler:           g.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if ts == nil {
		srv.Addr = addr
	}

	go func() {
		log.Infof("gateway: bridged actor is @%s@%s -> %s", user, host, g.actorID(user))
		if ts != nil {
			ln, lerr := ts.ListenFunnel("tcp", ":443")
			if lerr != nil {
				log.Fatalf("gateway: tailscale funnel: %v", lerr)
			}
			log.Infof("gateway: serving public https://%s via Tailscale Funnel", host)
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("gateway: serve: %v", err)
			}
			return
		}
		log.Infof("gateway: listening on %s, public https://%s", addr, host)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("gateway: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Infoln("gateway: shutting down...")
	appCancel()
	if nodeCli != nil {
		nodeCli.close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	if ts != nil {
		_ = ts.Close()
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
