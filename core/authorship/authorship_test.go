//nolint:all
package authorship

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
)

// stubStreamer mirrors MemberNode: paired device ids are the text form the pair
// handler stored, and NodeInfo.Aliases is the same text wrapped in WarpPeerID
// without decoding — which is why VerifyAuthor must not read that field.
type stubStreamer struct {
	ownNodeId warpnet.WarpPeerID
	devices   []string
}

func (s stubStreamer) GenericStream(string, stream.WarpRoute, any) ([]byte, error) {
	return nil, nil
}

func (s stubStreamer) PairedDeviceIDs() []string { return s.devices }

func (s stubStreamer) NodeInfo() warpnet.NodeInfo {
	info := warpnet.NodeInfo{ID: s.ownNodeId}
	for _, d := range s.devices {
		info.Aliases = append(info.Aliases, warpnet.WarpPeerID(d))
	}
	return info
}

func newPeer(t *testing.T) warpnet.WarpPeerID {
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

// TestVerifyAuthor covers the paired-device exception. A paired client dials
// this node with its own peer id while acting for the owner, so the connection
// says "device" where the actor's record says "this node".
func TestVerifyAuthor(t *testing.T) {
	var (
		ownNodeId = newPeer(t)
		device    = newPeer(t)
		stranger  = newPeer(t)
	)

	paired := stubStreamer{ownNodeId: ownNodeId, devices: []string{device.String()}}

	inbound := func(remote warpnet.WarpPeerID) warpnet.WarpStream {
		_, server := stream.NewLoopbackStream(ownNodeId, remote, "/test/route/0.0.0")
		t.Cleanup(func() { _ = server.Close() })
		return server
	}

	t.Run("the actor's own node is accepted, as before", func(t *testing.T) {
		if err := VerifyAuthor(paired, inbound(stranger), stranger.String()); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("a paired device may author for a user this node hosts", func(t *testing.T) {
		if err := VerifyAuthor(paired, inbound(device), ownNodeId.String()); err != nil {
			t.Fatalf("paired device rejected: %v", err)
		}
	})

	t.Run("a paired device may not author for a foreign node", func(t *testing.T) {
		err := VerifyAuthor(paired, inbound(device), stranger.String())
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("an unpaired peer may not author for this node", func(t *testing.T) {
		err := VerifyAuthor(paired, inbound(stranger), ownNodeId.String())
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("unpairing revokes it", func(t *testing.T) {
		unpaired := stubStreamer{ownNodeId: ownNodeId}
		err := VerifyAuthor(unpaired, inbound(device), ownNodeId.String())
		if !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("an empty actor node is never authored", func(t *testing.T) {
		if err := VerifyAuthor(paired, inbound(device), ""); !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})

	t.Run("a nil stream is never authored", func(t *testing.T) {
		if err := VerifyAuthor(paired, nil, ownNodeId.String()); !errors.Is(err, warpnet.ErrForeignAuthor) {
			t.Fatalf("expected %v, got %v", warpnet.ErrForeignAuthor, err)
		}
	})
}

// TestNodeInfoAliasesAreNotPeerIDs pins the reason VerifyAuthor reads
// PairedDeviceIDs rather than NodeInfo.Aliases: WarpPeerID is peer.ID, whose
// bytes are the binary multihash, so converting the stored base58 text to it
// yields a value that never equals the real peer id. A future fix that decodes
// aliases properly should delete this test — and may then simplify VerifyAuthor.
func TestNodeInfoAliasesAreNotPeerIDs(t *testing.T) {
	device := newPeer(t)

	rebuilt := warpnet.WarpPeerID(device.String())
	if rebuilt == device {
		t.Fatal("NodeInfo.Aliases now round-trips; VerifyAuthor can compare peer ids directly")
	}
	if string(rebuilt) != device.String() {
		t.Fatalf("alias text lost: %q", string(rebuilt))
	}
}
