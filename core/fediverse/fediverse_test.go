//nolint:all
package fediverse

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

	t.Run("creates one entry account per bridged network", func(t *testing.T) {
		repo := &stubSeeder{}
		SeedEntryUser(repo)

		require.Len(t, repo.created, 2)
		require.Empty(t, repo.updated)

		byId := map[string]domain.User{}
		for _, u := range repo.created {
			byId[u.Id] = u
		}
		require.Equal(t, MastodonNetwork, byId[EntryHandle].Network)
		require.Equal(t, gatewayNodeID, byId[EntryHandle].NodeId)
		// Threads serves no search and no follow graph, so without this seed the
		// Threads tab of the recommendations can never show anything.
		require.Equal(t, ThreadsNetwork, byId[ThreadsEntryHandle].Network)
		require.Equal(t, gatewayNodeID, byId[ThreadsEntryHandle].NodeId)
	})

	t.Run("falls back to update when an account already exists", func(t *testing.T) {
		repo := &stubSeeder{createErr: errors.New("already exists")}
		SeedEntryUser(repo)

		require.Len(t, repo.created, 2)
		require.Len(t, repo.updated, 2)
		updated := []string{repo.updated[0].Id, repo.updated[1].Id}
		require.ElementsMatch(t, []string{EntryHandle, ThreadsEntryHandle}, updated)
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

func TestIsBridged(t *testing.T) {
	for network, want := range map[string]bool{
		MastodonNetwork: true, ThreadsNetwork: true, "": false, "warpnet": false, "testnet": false,
	} {
		if got := IsBridged(network); got != want {
			t.Errorf("IsBridged(%q) = %v, want %v", network, got, want)
		}
	}
}
