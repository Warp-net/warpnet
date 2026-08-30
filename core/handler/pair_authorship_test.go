//nolint:all
package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/Warp-net/warpnet/core/authorship"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
)

type repoBackedStreamer struct {
	ownNodeId warpnet.WarpPeerID
	aliases   *database.AliasesRepo
}

func (s repoBackedStreamer) GenericStream(string, stream.WarpRoute, any) ([]byte, error) {
	return nil, nil
}

func (s repoBackedStreamer) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{ID: s.ownNodeId}
}

// Mirrors MemberNode.PairedDeviceIDs.
func (s repoBackedStreamer) PairedDeviceIDs() []string {
	ids, err := s.aliases.GetNodeIDs()
	if err != nil {
		return nil
	}
	return ids
}

type fixedToken struct{ token string }

func (f fixedToken) SessionToken() string { return f.token }

type pairAddrs struct{}

func (pairAddrs) PublicAddrs() []warpnet.WarpAddress { return nil }

func newTestPeer(t *testing.T) warpnet.WarpPeerID {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	id, err := warpnet.IDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("derive node id: %v", err)
	}
	return id
}

// TestPairThenAuthor walks the real chain a paired phone takes: the pair
// handler persists the device through the production AliasesRepo, and
// authorship.VerifyAuthor then reads it back through the same repo. It is the
// seam that silently broke when the device peer id was round-tripped through
// warpnet.WarpPeerID, so it is asserted against real storage rather than a stub.
func TestPairThenAuthor(t *testing.T) {
	db, err := local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Run("test", "test"); err != nil {
		t.Fatalf("run store: %v", err)
	}

	var (
		ownNodeId = newTestPeer(t)
		device    = newTestPeer(t)
		stranger  = newTestPeer(t)
		token     = "pairing-token"
	)

	aliases := database.NewAliasesRepo(db)
	streamer := repoBackedStreamer{ownNodeId: ownNodeId, aliases: aliases}

	inbound := func(remote warpnet.WarpPeerID, route warpnet.WarpProtocolID) warpnet.WarpStream {
		_, server := stream.NewLoopbackStream(ownNodeId, remote, route)
		t.Cleanup(func() { _ = server.Close() })
		return server
	}

	// Before pairing, the device is just another peer.
	if err := authorship.VerifyAuthor(
		streamer, inbound(device, "/test/route/0.0.0"), ownNodeId.String(),
	); err == nil {
		t.Fatal("an unpaired device must not author for this node")
	}

	// Pair it, exactly as the phone does.
	pair := StreamNodesPairingHandler(fixedToken{token}, aliases, pairAddrs{})
	if _, err := pair(
		marshal(t, domain.AuthNodeInfo{Token: token}),
		inbound(device, "/private/post/admin/pair/0.0.0"),
	); err != nil {
		t.Fatalf("pairing failed: %v", err)
	}

	t.Run("the paired device may now author for this node", func(t *testing.T) {
		if err := authorship.VerifyAuthor(
			streamer, inbound(device, "/test/route/0.0.0"), ownNodeId.String(),
		); err != nil {
			t.Fatalf("paired device rejected: %v", err)
		}
	})

	t.Run("another peer still may not", func(t *testing.T) {
		if err := authorship.VerifyAuthor(
			streamer, inbound(stranger, "/test/route/0.0.0"), ownNodeId.String(),
		); err == nil {
			t.Fatal("an unpaired peer must be rejected")
		}
	})

	t.Run("and not for a foreign node", func(t *testing.T) {
		if err := authorship.VerifyAuthor(
			streamer, inbound(device, "/test/route/0.0.0"), stranger.String(),
		); err == nil {
			t.Fatal("a paired device must not author for a third party")
		}
	})
}
