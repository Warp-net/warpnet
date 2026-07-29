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

// natlab is the node side of the NAT hole-punch harness. It builds a libp2p
// host from the production node.CommonOptions and reports DCUtR progress, so
// that topology.sh can assert a real hole punch happened between two peers
// sitting behind two independent MASQUERADE NATs.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/node"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/security"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	log "github.com/sirupsen/logrus"
)

// The lab runs on its own PSK so it can never talk to mainnet or testnet.
const (
	labNetwork = "natlab"
	labVersion = "0.0.1"
)

// Everything is configured through the environment on purpose: importing
// warpnet/config runs pflag.Parse() from its init(), which rejects any flag of
// ours before main() is even reached.
var (
	role         = envStr("NATLAB_ROLE", "")
	seed         = envStr("NATLAB_SEED", "")
	listenIP     = envStr("NATLAB_IP", "")
	listenPort   = envInt("NATLAB_PORT", 4001)
	relayAddr    = envStr("NATLAB_RELAY", "")
	targetID     = envStr("NATLAB_TARGET", "")
	dialCircuit  = envBool("NATLAB_DIAL_CIRCUIT")
	readyFile    = envStr("NATLAB_READY_FILE", "")
	waitFile     = envStr("NATLAB_WAIT_FILE", "")
	doneFile     = envStr("NATLAB_DONE_FILE", "")
	forcePrivate = envBool("NATLAB_FORCE_PRIVATE")
	timeout      = envDuration("NATLAB_TIMEOUT", 120*time.Second)
)

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(envStr(key, ""))
	if err != nil {
		return def
	}
	return v
}

func envBool(key string) bool {
	switch envStr(key, "") {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(envStr(key, ""))
	if err != nil {
		return def
	}
	return d
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.TimeOnly})

	if role == "print-id" {
		id, err := peerIDFromSeed(seed)
		if err != nil {
			fatal(err)
		}
		fmt.Println(id.String())
		return
	}

	if err := run(); err != nil {
		report("RESULT=FAIL", "role="+role, "err="+quote(err.Error()))
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch role {
	case "relay":
		return runRelay(ctx)
	case "peer":
		return runPeer(ctx)
	default:
		return fmt.Errorf("unknown role %q", role)
	}
}

// runRelay starts the public node: it is the circuit relay, the AutoNAT server
// and the only observer of the NATed peers' external addresses.
func runRelay(ctx context.Context) error {
	h, tracer, err := newHost(nil)
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()
	_ = tracer

	report("event=relay_up", "id="+h.ID().String(), "addrs="+quote(addrsString(h.Addrs())))

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	select {
	case <-interrupt:
	case <-ctx.Done():
	}
	return nil
}

// runPeer starts a NATed node: it reserves a slot on the relay, then either
// dials the other peer over the circuit or waits to be dialed. Either way the
// DCUtR exchange that follows must upgrade the connection to a direct one.
func runPeer(ctx context.Context) error {
	relayInfo, err := addrInfo(relayAddr)
	if err != nil {
		return fmt.Errorf("bad NATLAB_RELAY: %w", err)
	}
	target, err := peer.Decode(targetID)
	if err != nil {
		return fmt.Errorf("bad NATLAB_TARGET: %w", err)
	}

	h, tracer, err := newHost([]peer.AddrInfo{*relayInfo})
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()

	report("event=peer_up", "id="+h.ID().String(), "addrs="+quote(addrsString(h.Addrs())))

	h.Network().Notify(&connLogger{})

	if err := h.Connect(ctx, *relayInfo); err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	report("event=relay_connected", "relay="+relayInfo.ID.String())

	circuit, err := waitCircuitAddr(ctx, h)
	if err != nil {
		return err
	}
	report("event=reservation", "circuit="+quote(circuit))

	// The hole punch service registers its stream handler only once it has a
	// public address to offer (holepunch.Service.waitForPublicAddr). Until then
	// the peer answers "protocols not supported: [/libp2p/dcutr]" and the other
	// side's single punch attempt is wasted.
	if err := waitDCUtRReady(ctx, h); err != nil {
		return err
	}
	report("event=dcutr_ready")

	if readyFile != "" {
		if err := os.WriteFile(readyFile, []byte(circuit+"\n"), 0o600); err != nil {
			return fmt.Errorf("write ready file: %w", err)
		}
	}

	if dialCircuit {
		peerCircuit, err := waitForFile(ctx, waitFile)
		if err != nil {
			return err
		}
		info, err := addrInfo(peerCircuit)
		if err != nil {
			return fmt.Errorf("bad peer circuit addr: %w", err)
		}
		if info.ID != target {
			return fmt.Errorf("circuit addr is for %s, want %s", info.ID, target)
		}
		if err := h.Connect(ctx, *info); err != nil {
			return fmt.Errorf("connect over circuit: %w", err)
		}
		report("event=circuit_connected", "peer="+target.String())
	}

	direct, err := waitDirectConn(ctx, h, target)
	if err != nil {
		return err
	}
	report(
		"event=direct_conn",
		"peer="+target.String(),
		"local="+quote(direct.LocalMultiaddr().String()),
		"remote="+quote(direct.RemoteMultiaddr().String()),
	)

	// A direct connection whose remote address is public proves the packets
	// crossed the peer's NAT rather than taking a LAN shortcut.
	if !manet.IsPublicAddr(direct.RemoteMultiaddr()) {
		return fmt.Errorf("direct conn is not via a public address: %s", direct.RemoteMultiaddr())
	}

	if err := pingOver(ctx, h, target, direct); err != nil {
		return err
	}

	report("RESULT=PASS", "role=peer", "id="+h.ID().String(), "punched="+fmt.Sprint(tracer.punched()))

	// Whoever finishes first must not tear the punched connection down under
	// the other peer: closing the host closes the shared TCP connection, and
	// the peer still verifying it would see it vanish.
	if doneFile != "" {
		if dialCircuit {
			if err := os.WriteFile(doneFile, []byte("done\n"), 0o600); err != nil {
				return fmt.Errorf("write done file: %w", err)
			}
		} else if _, err := waitForFile(ctx, doneFile); err != nil {
			return fmt.Errorf("peer never confirmed the punch: %w", err)
		}
	}
	return nil
}

func newHost(relays []peer.AddrInfo) (warpnet.P2PNode, *hpTracer, error) {
	privKey, err := security.GenerateKeyFromSeed([]byte(seed))
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	selfID, err := warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, nil, fmt.Errorf("derive peer id: %w", err)
	}
	version, err := semver.NewVersion(labVersion)
	if err != nil {
		return nil, nil, err
	}
	psk, err := security.GeneratePSK(labNetwork, version)
	if err != nil {
		return nil, nil, fmt.Errorf("generate psk: %w", err)
	}

	tracer := &hpTracer{}

	opts := []libp2p.Option{
		node.WarpIdentity(privKey),
		libp2p.PrivateNetwork(warpnet.PSK(psk)),
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", listenIP, listenPort)),
	}
	if len(relays) > 0 {
		opts = append(opts, node.EnableAutoRelayWithStaticRelays(relays, selfID)())
	}
	opts = append(opts, node.CommonOptions...)
	// Re-apply with a tracer: CommonOptions enables hole punching without one,
	// and the option only overwrites HolePunchingOptions.
	opts = append(opts, libp2p.EnableHolePunching(holepunch.WithTracer(tracer)))
	if role == "relay" {
		// The circuit v2 hop service only starts once reachability is
		// confirmed public (relaysvc.reachabilityChanged). The lab relay does
		// sit on a public address, so state that instead of waiting for
		// AutoNAT to agree.
		opts = append(opts, libp2p.ForceReachabilityPublic())
	}
	if forcePrivate {
		opts = append(opts, libp2p.ForceReachabilityPrivate())
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("new host: %w", err)
	}
	return h, tracer, nil
}

// waitCircuitAddr waits until AutoNAT declares us private and AutoRelay has a
// confirmed reservation, which is what makes us dialable over the relay.
func waitCircuitAddr(ctx context.Context, h warpnet.P2PNode) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, a := range h.Addrs() {
			if !warpnet.IsRelayMultiaddress(a) {
				continue
			}
			return fmt.Sprintf("%s/p2p/%s", a, h.ID()), nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return "", fmt.Errorf("no relay reservation: %w", ctx.Err())
		}
	}
}

// waitDCUtRReady waits until our own host answers the DCUtR protocol, which is
// the observable proof that hole punching has a public address to work with.
func waitDCUtRReady(ctx context.Context, h warpnet.P2PNode) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, p := range h.Mux().Protocols() {
			if p == holepunch.Protocol {
				return nil
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("hole punch service never got a public address: %w", ctx.Err())
		}
	}
}

// waitDirectConn waits for a connection to target that is neither limited nor
// routed through a relay, i.e. the result of a successful hole punch.
func waitDirectConn(ctx context.Context, h warpnet.P2PNode, target peer.ID) (network.Conn, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, c := range h.Network().ConnsToPeer(target) {
			if c.Stat().Limited || warpnet.IsRelayMultiaddress(c.RemoteMultiaddr()) {
				continue
			}
			return c, nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			for _, c := range h.Network().Conns() {
				report(
					"event=conn_dump",
					"peer="+c.RemotePeer().String(),
					"remote="+quote(c.RemoteMultiaddr().String()),
					"limited="+fmt.Sprint(c.Stat().Limited),
					"direction="+c.Stat().Direction.String(),
				)
			}
			return nil, fmt.Errorf("no direct connection to %s: %w", target, ctx.Err())
		}
	}
}

// pingOver pings target and verifies libp2p picked the hole-punched connection.
func pingOver(ctx context.Context, h warpnet.P2PNode, target peer.ID, direct network.Conn) error {
	pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	s, err := h.NewStream(pingCtx, target, ping.ID)
	if err != nil {
		return fmt.Errorf("open ping stream: %w", err)
	}
	used := s.Conn().RemoteMultiaddr().String()
	_ = s.Close()
	if used != direct.RemoteMultiaddr().String() {
		return fmt.Errorf("ping stream used %s, not the direct conn %s", used, direct.RemoteMultiaddr())
	}

	res := <-ping.Ping(pingCtx, h, target)
	if res.Error != nil {
		return fmt.Errorf("ping over direct conn: %w", res.Error)
	}
	report("event=ping_ok", "peer="+target.String(), "rtt="+res.RTT.String(), "via="+quote(used))
	return nil
}

// connLogger traces the full lifecycle of every connection, which is what tells
// a punch that never happened apart from one that happened and was then torn
// down.
type connLogger struct{}

func (connLogger) Listen(network.Network, ma.Multiaddr)      {}
func (connLogger) ListenClose(network.Network, ma.Multiaddr) {}

func (connLogger) Connected(_ network.Network, c network.Conn) {
	report(
		"event=conn_open",
		"peer="+c.RemotePeer().String(),
		"remote="+quote(c.RemoteMultiaddr().String()),
		"limited="+fmt.Sprint(c.Stat().Limited),
		"direction="+c.Stat().Direction.String(),
	)
}

func (connLogger) Disconnected(_ network.Network, c network.Conn) {
	report(
		"event=conn_close",
		"peer="+c.RemotePeer().String(),
		"remote="+quote(c.RemoteMultiaddr().String()),
		"limited="+fmt.Sprint(c.Stat().Limited),
		"direction="+c.Stat().Direction.String(),
	)
}

// hpTracer records DCUtR events so the log carries direct evidence of the
// punch, not just its end result.
type hpTracer struct {
	success atomic.Bool
}

func (t *hpTracer) Trace(evt *holepunch.Event) {
	payload, err := json.Marshal(evt.Evt)
	if err != nil {
		payload = []byte(`{}`)
	}
	if end, ok := evt.Evt.(*holepunch.EndHolePunchEvt); ok && end.Success {
		t.success.Store(true)
	}
	report("event=dcutr", "type="+evt.Type, "remote="+evt.Remote.String(), "evt="+quote(string(payload)))
}

func (t *hpTracer) punched() bool { return t.success.Load() }

func peerIDFromSeed(s string) (peer.ID, error) {
	privKey, err := security.GenerateKeyFromSeed([]byte(s))
	if err != nil {
		return "", err
	}
	return warpnet.IDFromPublicKey(privKey.Public().(ed25519.PublicKey))
}

func addrInfo(s string) (*peer.AddrInfo, error) {
	maddr, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}
	return peer.AddrInfoFromP2pAddr(maddr)
}

func waitForFile(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("no -wait-file given")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		bt, err := os.ReadFile(path)
		if err == nil && len(strings.TrimSpace(string(bt))) > 0 {
			return strings.TrimSpace(string(bt)), nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return "", fmt.Errorf("waiting for %s: %w", path, ctx.Err())
		}
	}
}

func addrsString(addrs []ma.Multiaddr) string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}
	return strings.Join(out, ",")
}

// report writes a machine-readable line that topology.sh greps for.
func report(fields ...string) {
	fmt.Printf("NATLAB %s\n", strings.Join(fields, " "))
	_ = os.Stdout.Sync()
}

func quote(s string) string { return "'" + s + "'" }

func fatal(err error) {
	report("RESULT=FAIL", "err="+quote(err.Error()))
	os.Exit(1)
}
