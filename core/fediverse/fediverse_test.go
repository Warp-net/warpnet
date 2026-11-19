//nolint:all
package fediverse

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/config"
	local_store "github.com/Warp-net/warpnet/database/local-store"
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
		MastodonNetwork: true, ThreadsNetwork: true, "nostr": true, "any-other": true,
		"": false, "warpnet": false, "mainnet": false, "testnet": false,
	} {
		if got := IsBridged(network); got != want {
			t.Errorf("IsBridged(%q) = %v, want %v", network, got, want)
		}
	}
}

func TestIsBridgedOwnPrivateNetwork(t *testing.T) {
	original := config.Config().Node.Network
	t.Cleanup(func() { config.SetNetwork(original) })

	config.SetNetwork("private")
	require.False(t, IsBridged("private"), "this node's own network is native")
	require.True(t, IsBridged("nostr"))
}

func TestIsFederatedWarpnet(t *testing.T) {
	for id, want := range map[string]bool{
		"01KTRA1QJ8M2W7Y4ZB6C9D3E5F@warpnet-gw.example":  true,
		"@01ktra1qj8m2w7y4zb6c9d3e5f@warpnet-gw.example": true,
		"01KTRA1QJ8M2W7Y4ZB6C9D3E5F":                     false,
		"01KTRA1QJ8M2W7Y4ZB6C9D3E5F@":                    false,
		EntryHandle:                                      false,
		ThreadsEntryHandle:                               false,
	} {
		if got := IsFederatedWarpnet(domain.User{Id: id}); got != want {
			t.Errorf("IsFederatedWarpnet(%q) = %v, want %v", id, got, want)
		}
	}
}

type stubMigrator struct {
	pages   [][]domain.User
	updated []domain.User
}

func (s *stubMigrator) List(_ *uint64, cursor *string) ([]domain.User, string, error) {
	i := 0
	if *cursor != "" {
		i = int((*cursor)[0] - '0')
	}
	if i >= len(s.pages) {
		return nil, local_store.EndCursor, nil
	}
	next := local_store.EndCursor
	if i+1 < len(s.pages) {
		next = string(rune('0' + i + 1))
	}
	return s.pages[i], next, nil
}

func (s *stubMigrator) Update(_ string, u domain.User) (domain.User, error) {
	s.updated = append(s.updated, u)
	return u, nil
}

func TestMoveOffLegacyGateway(t *testing.T) {
	repo := &stubMigrator{pages: [][]domain.User{
		{{Id: "a@mastodon.social", NodeId: legacyGatewayNodeID}, {Id: "native", NodeId: "12D3KooWOther"}},
		{{Id: "b@threads.net", NodeId: legacyGatewayNodeID}},
	}}
	MoveOffLegacyGateway(repo)
	require.Len(t, repo.updated, 2)
	for _, u := range repo.updated {
		require.Equal(t, GatewayNodeID(), u.NodeId)
	}
}

func TestLegacyGatewayIDIsNotAccepted(t *testing.T) {
	original := GatewayNodeID()
	SetGatewayNodeID(legacyGatewayNodeID)
	require.Equal(t, original, GatewayNodeID(), "the legacy gateway id is claimable by anyone")
}
