//go:build mobile

package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// freshKey returns a 64-byte libp2p Ed25519 private key (seed || pub).
func freshKey(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return priv
}

// freshPSK returns a 32-byte PSK; valid length for libp2p PrivateNetwork.
func freshPSK(t *testing.T) []byte {
	t.Helper()
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}
	return psk
}

// newClient requires live bootstrap reachability; these tests exercise the
// preflight validation branches so they don't need a network.

func TestNewClientRejectsMissingPSK(t *testing.T) {
	_, err := newClient(freshKey(t), nil, "testnet", []string{"addr"})
	if err == nil {
		t.Fatal("expected psk required error")
	}
}

func TestNewClientRejectsMissingBootstrap(t *testing.T) {
	_, err := newClient(freshKey(t), freshPSK(t), "testnet", nil)
	if err == nil {
		t.Fatal("expected bootstrap required error")
	}
}

func TestNewClientRejectsInvalidPrivKey(t *testing.T) {
	_, err := newClient([]byte{0x00}, freshPSK(t), "testnet", []string{"addr"})
	if err == nil {
		t.Fatal("expected invalid priv key error")
	}
}

// TestClientNodeSign_VerifiesAgainstLibp2pPubKey ensures that a body signed by
// clientNode.sign verifies against the matching ed25519 public key the desktop
// side derives from the libp2p peer ID (see warpnet.FromIDToPubKey).
func TestClientNodeSign_VerifiesAgainstLibp2pPubKey(t *testing.T) {
	priv := freshKey(t)
	pk, err := crypto.UnmarshalEd25519PrivateKey(priv)
	if err != nil {
		t.Fatalf("UnmarshalEd25519PrivateKey: %v", err)
	}
	cn := &clientNode{privKey: pk}

	body := []byte(`{"hello":"world"}`)
	sigB64, err := cn.sign(body)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	pubRaw, err := pk.GetPublic().Raw()
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if !ed25519.Verify(pubRaw, body, sig) {
		t.Fatal("signature did not verify against ed25519 public key")
	}
}

func TestClientNodeSign_NoKey(t *testing.T) {
	cn := &clientNode{}
	if _, err := cn.sign([]byte("body")); err == nil {
		t.Fatal("expected error when private key not set")
	}
}

type stubDeadliner struct{ deadlines int }

func (s *stubDeadliner) SetReadDeadline(time.Time) error {
	s.deadlines++
	return nil
}

func TestReadResponse_ReadsWholeBody(t *testing.T) {
	deadliner := &stubDeadliner{}

	got, err := readResponse(strings.NewReader(`{"ok":true}`), deadliner)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("unexpected response: %q", got)
	}
	if deadliner.deadlines == 0 {
		t.Fatal("expected a read deadline to be set")
	}
}

func TestReadResponse_StopsAtMaxSize(t *testing.T) {
	oversized := strings.NewReader(strings.Repeat("a", maxResponseSize+1))

	_, err := readResponse(oversized, &stubDeadliner{})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected an oversize error, got: %v", err)
	}
}

func TestLanFirstDialRanker(t *testing.T) {
	raw := []string{
		"/ip4/172.17.0.1/tcp/4001",
		"/ip4/192.168.1.138/tcp/4001",
		"/ip6/::1/tcp/4001",
		"/ip4/127.0.0.1/tcp/4001",
		"/ip4/95.164.7.11/tcp/4001",
		"/ip4/207.154.221.44/tcp/4001/p2p-circuit",
	}
	addrs := make([]multiaddr.Multiaddr, 0, len(raw))
	for _, s := range raw {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			t.Fatalf("multiaddr %q: %v", s, err)
		}
		addrs = append(addrs, addr)
	}

	for _, ranked := range lanFirstDialRanker(addrs) {
		isPrivate := manet.IsPrivateAddr(ranked.Addr) && !isRelayAddr(ranked.Addr)
		if isPrivate && ranked.Delay != 0 {
			t.Fatalf("%s is on the local network and must be dialled first, got %s", ranked.Addr, ranked.Delay)
		}
		if !isPrivate && ranked.Delay < lanHeadStart {
			t.Fatalf("%s must not outrun the local network, got %s", ranked.Addr, ranked.Delay)
		}
	}
}

func TestLanFirstDialRankerFallsBackToDefault(t *testing.T) {
	addr, err := multiaddr.NewMultiaddr("/ip4/95.164.7.11/tcp/4001")
	if err != nil {
		t.Fatalf("multiaddr: %v", err)
	}
	ranked := lanFirstDialRanker([]multiaddr.Multiaddr{addr})
	if len(ranked) != 1 || ranked[0].Delay != 0 {
		t.Fatalf("expected the only address dialled immediately, got %v", ranked)
	}
}
