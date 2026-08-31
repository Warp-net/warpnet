//nolint:all
package mastodon

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/stretchr/testify/require"
)

type stubSeeder struct {
	createErr error
	created   []domain.User
	updated   []domain.User
}

func (s *stubSeeder) Create(user domain.User) (domain.User, error) {
	s.created = append(s.created, user)
	return user, s.createErr
}

func (s *stubSeeder) Update(userId string, newUser domain.User) (domain.User, error) {
	s.updated = append(s.updated, newUser)
	return newUser, nil
}

func TestGatewayNodeID(t *testing.T) {
	original := GatewayNodeID()
	t.Cleanup(func() { gatewayNodeID = original })

	require.Equal(t, DefaultGatewayNodeID, original)

	SetGatewayNodeID("")
	require.Equal(t, DefaultGatewayNodeID, GatewayNodeID(), "an empty id must not clear the default")

	SetGatewayNodeID("12D3KooWCustomGateway")
	require.Equal(t, "12D3KooWCustomGateway", GatewayNodeID())
}

func TestSeedEntryUser(t *testing.T) {
	original := gatewayNodeID
	t.Cleanup(func() { gatewayNodeID = original })

	t.Run("creates the entry account", func(t *testing.T) {
		repo := &stubSeeder{}
		SeedEntryUser(repo)

		require.Len(t, repo.created, 1)
		require.Empty(t, repo.updated)

		u := repo.created[0]
		require.Equal(t, EntryHandle, u.Id)
		require.Equal(t, Network, u.Network)
		require.Equal(t, gatewayNodeID, u.NodeId)
	})

	t.Run("falls back to update when the account already exists", func(t *testing.T) {
		repo := &stubSeeder{createErr: errors.New("already exists")}
		SeedEntryUser(repo)

		require.Len(t, repo.created, 1)
		require.Len(t, repo.updated, 1)
		require.Equal(t, EntryHandle, repo.updated[0].Id)
	})

	t.Run("uses the configured gateway", func(t *testing.T) {
		SetGatewayNodeID("12D3KooWAnotherGateway")
		repo := &stubSeeder{}
		SeedEntryUser(repo)
		require.Equal(t, "12D3KooWAnotherGateway", repo.created[0].NodeId)
	})
}

func TestErrNotSupported(t *testing.T) {
	require.EqualError(t, ErrNotSupported, "not supported functionality")
}
