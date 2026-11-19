//nolint:all
package authorship

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/stretchr/testify/require"
)

type stubUserStore struct {
	getFn    func(userId string) (domain.User, error)
	createFn func(user domain.User) (domain.User, error)
	created  []domain.User
}

func (s *stubUserStore) Get(userId string) (domain.User, error) {
	if s.getFn != nil {
		return s.getFn(userId)
	}
	return domain.User{}, database.ErrUserNotFound
}

func (s *stubUserStore) Create(user domain.User) (domain.User, error) {
	s.created = append(s.created, user)
	if s.createFn != nil {
		return s.createFn(user)
	}
	return user, nil
}

type stubStreamer struct {
	fn func(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

func (s stubStreamer) GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error) {
	if s.fn != nil {
		return s.fn(nodeId, path, data)
	}
	return nil, nil
}

// peerStream mints a real peer id and a loopback stream whose remote peer is it.
func peerStream(t *testing.T) (warpnet.WarpPeerID, warpnet.WarpStream) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	id, err := warpnet.IDFromPublicKey(pub)
	require.NoError(t, err)
	_, server := stream.NewLoopbackStream(id, id, "/test/route/0.0.0")
	return id, server
}

func TestFetchActor(t *testing.T) {
	const actorId = "actor-1"
	nodeId, s := peerStream(t)

	t.Run("known actor is returned from storage", func(t *testing.T) {
		repo := &stubUserStore{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		actor, err := FetchActor(repo, stubStreamer{}, s, actorId)
		require.NoError(t, err)
		require.Equal(t, actorId, actor.Id)
		require.Empty(t, repo.created)
	})

	t.Run("a non-not-found lookup failure surfaces", func(t *testing.T) {
		lookupErr := errors.New("store down")
		repo := &stubUserStore{getFn: func(string) (domain.User, error) {
			return domain.User{}, lookupErr
		}}
		_, err := FetchActor(repo, stubStreamer{}, s, actorId)
		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("no stream means no remote lookup", func(t *testing.T) {
		_, err := FetchActor(&stubUserStore{}, stubStreamer{}, nil, actorId)
		require.ErrorIs(t, err, database.ErrUserNotFound)
	})

	t.Run("stream failure surfaces", func(t *testing.T) {
		streamErr := errors.New("offline")
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return nil, streamErr
		}}
		_, err := FetchActor(&stubUserStore{}, streamer, s, actorId)
		require.ErrorIs(t, err, streamErr)
	})

	t.Run("garbage response surfaces", func(t *testing.T) {
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return []byte("{"), nil
		}}
		_, err := FetchActor(&stubUserStore{}, streamer, s, actorId)
		require.Error(t, err)
	})

	t.Run("an empty remote record is still not found", func(t *testing.T) {
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{})
		}}
		_, err := FetchActor(&stubUserStore{}, streamer, s, actorId)
		require.ErrorIs(t, err, database.ErrUserNotFound)
	})

	t.Run("a fetched actor is cached locally", func(t *testing.T) {
		repo := &stubUserStore{}
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{Id: actorId, NodeId: nodeId.String()})
		}}
		actor, err := FetchActor(repo, streamer, s, actorId)
		require.NoError(t, err)
		require.Equal(t, actorId, actor.Id)
		require.Len(t, repo.created, 1)
	})

	t.Run("an already-existing cache entry is not an error", func(t *testing.T) {
		repo := &stubUserStore{createFn: func(domain.User) (domain.User, error) {
			return domain.User{}, database.ErrUserAlreadyExists
		}}
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{Id: actorId, NodeId: nodeId.String()})
		}}
		_, err := FetchActor(repo, streamer, s, actorId)
		require.NoError(t, err)
	})

	t.Run("a failing cache write surfaces", func(t *testing.T) {
		createErr := errors.New("write down")
		repo := &stubUserStore{createFn: func(domain.User) (domain.User, error) {
			return domain.User{}, createErr
		}}
		streamer := stubStreamer{fn: func(string, stream.WarpRoute, any) ([]byte, error) {
			return json.Marshal(domain.User{Id: actorId, NodeId: nodeId.String()})
		}}
		_, err := FetchActor(repo, streamer, s, actorId)
		require.ErrorIs(t, err, createErr)
	})
}

func TestVerifyActor(t *testing.T) {
	const actorId = "actor-1"
	nodeId, s := peerStream(t)

	t.Run("actor on the sending node is accepted", func(t *testing.T) {
		repo := &stubUserStore{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: nodeId.String()}, nil
		}}
		actor, err := VerifyActor(repo, stubStreamer{}, s, actorId)
		require.NoError(t, err)
		require.Equal(t, actorId, actor.Id)
	})

	t.Run("actor on another node is rejected", func(t *testing.T) {
		repo := &stubUserStore{getFn: func(id string) (domain.User, error) {
			return domain.User{Id: id, NodeId: "somebody-else"}, nil
		}}
		_, err := VerifyActor(repo, stubStreamer{}, s, actorId)
		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
	})

	t.Run("an unresolvable actor is rejected", func(t *testing.T) {
		_, err := VerifyActor(&stubUserStore{}, stubStreamer{}, nil, actorId)
		require.ErrorIs(t, err, warpnet.ErrForeignAuthor)
	})
}
