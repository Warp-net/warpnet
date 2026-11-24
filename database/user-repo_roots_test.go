//nolint:all
package database

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/bridge"
	"github.com/Warp-net/warpnet/core/warpnet"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

const (
	noneRoot     = "/USERS/USER/None/"
	warpnetRoot  = "/USERS/USER/warpnet//"
	mastodonRoot = "/USERS/USER/mastodon//"
	threadsRoot  = "/USERS/USER/threads//"
	nostrRoot    = "/USERS/USER/nostr//"
	noneMarker   = "/USERS/USER/None"
)

func putRowsIn(t *testing.T, repo *UserRepo, root string, users ...domain.User) {
	t.Helper()
	txn, err := repo.db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()
	for _, u := range users {
		data, err := json.Marshal(u)
		require.NoError(t, err)
		require.NoError(t, txn.Set(userRowKeyIn(root, u.Id), data))
	}
	require.NoError(t, txn.Commit())
}

func putStoredUnderNone(t *testing.T, repo *UserRepo, u domain.User) {
	t.Helper()
	data, err := json.Marshal(u)
	require.NoError(t, err)
	fixed := local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID(noneNamespace).
		AddRange(local_store.FixedRangeKey).
		AddParentId(u.Id).
		Build()
	txn, err := repo.db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()
	require.NoError(t, txn.Set(userRowKey(u.Id), data))
	require.NoError(t, txn.Set(fixed, userRowKey(u.Id).Bytes()))
	require.NoError(t, txn.Commit())
}

func createAll(t *testing.T, repo *UserRepo, users ...domain.User) {
	t.Helper()
	for _, u := range users {
		_, err := repo.Create(u)
		require.NoError(t, err, u.Id)
	}
}

func bridged(network, id string) domain.User {
	return domain.User{Id: id, Username: id, NodeId: "gateway", Network: network}
}

func requireOnlyNetwork(t *testing.T, network string, users []domain.User) {
	t.Helper()
	for _, u := range users {
		require.Equal(t, network, resolveNetwork(u), "%s leaked into %q", u.Id, network)
	}
}

func TestUserRootNamespaces(t *testing.T) {
	for _, network := range []string{warpnet.WarpnetName, bridge.MastodonNetwork, bridge.ThreadsNetwork, bridge.NostrNetwork} {
		require.Contains(t, networkNamespaces, network, "every recommended network needs a root")
	}
	roots := []string{noneNamespace}
	for network, root := range networkNamespaces {
		require.True(t, strings.HasSuffix(root, "/"), "%s root %q must end its segment", network, root)
		require.Equal(t, network+"/", root)
		roots = append(roots, root)
	}
	for _, a := range roots {
		for _, b := range roots {
			if a != b {
				require.False(t, strings.HasPrefix(b, a), "root %q would catch root %q", a, b)
			}
		}
	}
}

func TestCreatePicksTheRoot(t *testing.T) {
	native := func() string { return ulid.Make().String() }
	cases := []struct {
		name, network, id, root string
	}{
		{"untagged ULID", "", native(), noneRoot},
		{"untagged handle", "", "u1@nonet.example", noneRoot},
		{"warpnet tag", warpnet.WarpnetName, native(), warpnetRoot},
		{"warpnet tag on a handle", warpnet.WarpnetName, "u2@nonet.example", warpnetRoot},
		{"testnet tag", "testnet", native(), warpnetRoot},
		{"mainnet tag", "mainnet", native(), warpnetRoot},
		{"mastodon", bridge.MastodonNetwork, "a@mastodon.social", mastodonRoot},
		{"threads", bridge.ThreadsNetwork, "b@threads.net", threadsRoot},
		{"nostr", bridge.NostrNetwork, "npub1a@nostr.com", nostrRoot},
		{"a network with no root", "bluesky", "c@bsky.social", noneRoot},
		{"a known network in capitals", "Mastodon", "d@mastodon.social", noneRoot},
		{"a known network shouting", "NOSTR", "npub1b@nostr.com", noneRoot},
		{"a known network padded", " mastodon ", "e@mastodon.social", noneRoot},
		{"a known network with a slash", "mastodon/", "f@mastodon.social", noneRoot},
		{"a network that walks the keyspace", "../nostr", "g@nostr.com", noneRoot},
		{"a network named None", "None", "h@none.example", noneRoot},
		{"a network named none", "none", "i@none.example", noneRoot},
		{"a network that is a slash", "/", "j@slash.example", noneRoot},
	}
	repo := newWhoToFollowRepo(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := repo.Create(domain.User{Id: tc.id, NodeId: "n-" + tc.id, Network: tc.network})
			require.NoError(t, err)
			require.Equal(t, tc.id, created.Id)
			require.True(t, strings.HasPrefix(storedKeyOf(t, repo, tc.id), tc.root), storedKeyOf(t, repo, tc.id))

			got, err := repo.Get(tc.id)
			require.NoError(t, err)
			require.Equal(t, tc.id, got.Id)
			require.Equal(t, tc.network, got.Network, "the stored tag is the one given")
		})
	}

	t.Run("the RTT range stays inside the root", func(t *testing.T) {
		for rtt, range_ := range map[int64]string{0: "125000", 7: "7", math.MaxInt64: fmt.Sprint(int64(math.MaxInt64)), -1: "-1"} {
			id := fmt.Sprintf("rtt%d@mastodon.social", rtt)
			_, err := repo.Create(domain.User{Id: id, NodeId: "n-" + id, Network: bridge.MastodonNetwork, RoundTripTime: rtt})
			require.NoError(t, err)
			require.Equal(t, mastodonRoot+range_+"/"+id, storedKeyOf(t, repo, id))
		}
	})
}

func TestCreateKeepsOneRowPerId(t *testing.T) {
	t.Run("a second create under another network is refused", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "a@x.example"))
		_, err := repo.Create(bridged(bridge.ThreadsNetwork, "a@x.example"))
		require.ErrorIs(t, err, ErrUserAlreadyExists)
		require.True(t, strings.HasPrefix(storedKeyOf(t, repo, "a@x.example"), mastodonRoot))

		users, _, err := repo.WhoToFollow(bridge.ThreadsNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
	})

	t.Run("a deleted user can come back under another root", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "a@x.example"))
		require.NoError(t, repo.Delete("a@x.example"))
		createAll(t, repo, bridged(bridge.NostrNetwork, "a@x.example"))
		require.True(t, strings.HasPrefix(storedKeyOf(t, repo, "a@x.example"), nostrRoot))

		masto, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, masto)
		nostr, _, err := repo.WhoToFollow(bridge.NostrNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"a@x.example"}, idsOfUsers(nostr))
	})

	t.Run("an empty id is refused before any root is picked", func(t *testing.T) {
		_, err := newWhoToFollowRepo(t).Create(domain.User{Network: bridge.NostrNetwork})
		require.Error(t, err)
	})
}

func TestRowsUnderNetworkRootsAnswerEveryLookup(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	native := ulid.Make().String()
	createAll(t, repo,
		bridged(bridge.MastodonNetwork, "a@mastodon.social"),
		bridged(bridge.ThreadsNetwork, "b@threads.net"),
		domain.User{Id: "npub1c@nostr.com", NodeId: "nostr-node", Network: bridge.NostrNetwork},
		domain.User{Id: native, NodeId: "warp-node", Network: "testnet"},
		domain.User{Id: "d@nonet.example", NodeId: "none-node"},
	)

	t.Run("GetByNodeID finds a row under a network root", func(t *testing.T) {
		got, err := repo.GetByNodeID("nostr-node")
		require.NoError(t, err)
		require.Equal(t, "npub1c@nostr.com", got.Id)
		got, err = repo.GetByNodeID("warp-node")
		require.NoError(t, err)
		require.Equal(t, native, got.Id)
	})

	t.Run("GetBatch mixes roots and skips the missing", func(t *testing.T) {
		got, err := repo.GetBatch("b@threads.net", "missing@nowhere", native, "d@nonet.example")
		require.NoError(t, err)
		require.Equal(t, []string{"b@threads.net", native, "d@nonet.example"}, idsOfUsers(got))
	})

	t.Run("Update rewrites the row in place", func(t *testing.T) {
		before := storedKeyOf(t, repo, "b@threads.net")
		_, err := repo.Update("b@threads.net", domain.User{Bio: "new bio", RoundTripTime: 3})
		require.NoError(t, err)
		require.Equal(t, before, storedKeyOf(t, repo, "b@threads.net"))
		got, err := repo.Get("b@threads.net")
		require.NoError(t, err)
		require.Equal(t, "new bio", got.Bio)
	})

	t.Run("going offline and back moves a user out of and into its page", func(t *testing.T) {
		_, err := repo.Update("a@mastodon.social", domain.User{IsOffline: true})
		require.NoError(t, err)
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)

		_, err = repo.Update("a@mastodon.social", domain.User{IsOffline: false})
		require.NoError(t, err)
		users, _, err = repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"a@mastodon.social"}, idsOfUsers(users))
	})

	t.Run("an untagged user tagged later moves to its network's root", func(t *testing.T) {
		_, err := repo.Update("d@nonet.example", domain.User{Network: bridge.MastodonNetwork})
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(storedKeyOf(t, repo, "d@nonet.example"), mastodonRoot))
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a@mastodon.social", "d@nonet.example"}, idsOfUsers(users))
	})

	t.Run("Delete clears the row, the id and the node index", func(t *testing.T) {
		require.NoError(t, repo.Delete("npub1c@nostr.com"))
		_, err := repo.Get("npub1c@nostr.com")
		require.ErrorIs(t, err, ErrUserNotFound)
		_, err = repo.GetByNodeID("nostr-node")
		require.ErrorIs(t, err, ErrUserNotFound)
		users, _, err := repo.WhoToFollow(bridge.NostrNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.NoError(t, repo.Delete("npub1c@nostr.com"), "deleting twice is a no-op")
	})
}

func TestTTLRowsExpireFromTheirRoot(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	_, err := repo.CreateWithTTL(bridged(bridge.ThreadsNetwork, "brief@threads.net"), time.Second)
	require.NoError(t, err)
	createAll(t, repo, bridged(bridge.ThreadsNetwork, "stays@threads.net"))
	require.True(t, strings.HasPrefix(storedKeyOf(t, repo, "brief@threads.net"), threadsRoot))

	require.Eventually(t, func() bool {
		_, err := repo.Get("brief@threads.net")
		return errors.Is(err, ErrUserNotFound)
	}, 5*time.Second, 100*time.Millisecond)

	users, _, err := repo.WhoToFollow(bridge.ThreadsNetwork, nil, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"stays@threads.net"}, idsOfUsers(users))
}

func TestListAndSearchCoverEveryRoot(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	native := ulid.Make().String()
	all := []domain.User{
		bridged(bridge.MastodonNetwork, "a@mastodon.social"),
		bridged(bridge.MastodonNetwork, "b@mastodon.social"),
		bridged(bridge.ThreadsNetwork, "c@threads.net"),
		bridged(bridge.NostrNetwork, "npub1d@nostr.com"),
		bridged("bluesky", "e@bsky.social"),
		{Id: native, Username: "native", NodeId: "warp-node", Network: warpnet.WarpnetName},
		{Id: "f@nonet.example", Username: "untagged", NodeId: "none-node"},
	}
	createAll(t, repo, all...)
	putUsers(t, repo, domain.User{Id: "g@mastodon.social", Username: "old row", Network: bridge.MastodonNetwork})
	want := append(idsOfUsers(all), "g@mastodon.social")

	t.Run("an empty store lists nothing", func(t *testing.T) {
		users, cur, err := newWhoToFollowRepo(t).List(nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("List returns every row exactly once and no index keys", func(t *testing.T) {
		limit := uint64(100)
		users, cur, err := repo.List(&limit, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, want, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, cur)
	})

	for _, size := range []uint64{1, 2, 3, 7} {
		t.Run(fmt.Sprintf("List pages of %d cross the roots without repeats", size), func(t *testing.T) {
			seen := map[string]int{}
			cursor := ""
			for range 100 {
				limit := size
				users, next, err := repo.List(&limit, &cursor)
				require.NoError(t, err)
				for _, u := range users {
					seen[u.Id]++
				}
				if next == local_store.EndCursor {
					break
				}
				cursor = next
			}
			require.Len(t, seen, len(want))
			for id, n := range seen {
				require.Equal(t, 1, n, id)
			}
		})
	}

	t.Run("Search finds matches in every root", func(t *testing.T) {
		limit := uint64(100)
		hits, _, err := repo.Search("gateway", &limit, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a@mastodon.social", "b@mastodon.social", "c@threads.net", "npub1d@nostr.com", "e@bsky.social"}, idsOfUsers(hits))

		for query, id := range map[string]string{"native": native, "untagged": "f@nonet.example", "old row": "g@mastodon.social", "npub1d": "npub1d@nostr.com"} {
			hits, _, err := repo.Search(query, &limit, nil)
			require.NoError(t, err)
			require.Equal(t, []string{id}, idsOfUsers(hits), query)
		}
	})

	t.Run("Search pages across roots without repeats", func(t *testing.T) {
		seen := map[string]int{}
		cursor := ""
		for range 100 {
			limit := uint64(1)
			hits, next, err := repo.Search("gateway", &limit, &cursor)
			require.NoError(t, err)
			for _, u := range hits {
				seen[u.Id]++
			}
			if next == local_store.EndCursor {
				break
			}
			cursor = next
		}
		require.Len(t, seen, 5)
	})

	t.Run("Search for nothing that exists ends at once", func(t *testing.T) {
		hits, cur, err := repo.Search("no-such-user-anywhere", nil, nil)
		require.NoError(t, err)
		require.Empty(t, hits)
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("a corrupt row under a network root fails List and Search", func(t *testing.T) {
		broken := newWhoToFollowRepo(t)
		createAll(t, broken, bridged(bridge.MastodonNetwork, "ok@mastodon.social"))
		txn, err := broken.db.NewTxn()
		require.NoError(t, err)
		require.NoError(t, txn.Set(userRowKeyIn(nostrNamespace, "broken"), []byte("{")))
		require.NoError(t, txn.Commit())

		limit := uint64(100)
		_, _, err = broken.List(&limit, nil)
		require.Error(t, err)
		_, _, err = broken.Search("ok", &limit, nil)
		require.Error(t, err)
	})
}

func TestWhoToFollowAcrossRoots(t *testing.T) {
	seed := func(t *testing.T) (*UserRepo, string) {
		t.Helper()
		repo := newWhoToFollowRepo(t)
		native := ulid.Make().String()
		createAll(t, repo,
			bridged(bridge.MastodonNetwork, "m1@mastodon.social"),
			bridged(bridge.MastodonNetwork, "m2@mastodon.social"),
			bridged(bridge.ThreadsNetwork, "t1@threads.net"),
			bridged(bridge.NostrNetwork, "npub1n@nostr.com"),
			bridged("bluesky", "b1@bsky.social"),
			domain.User{Id: native, NodeId: "warp-node", Network: "testnet"},
			domain.User{Id: "u1@nonet.example", NodeId: "u-node"},
		)
		putUsers(t, repo,
			domain.User{Id: "m0@mastodon.social", Network: bridge.MastodonNetwork},
			domain.User{Id: "t0@threads.net", Network: bridge.ThreadsNetwork},
			domain.User{Id: ulid.Make().String()},
		)
		return repo, native
	}

	t.Run("every network sees its own root, then its None rows, and nothing else", func(t *testing.T) {
		repo, native := seed(t)
		for network, want := range map[string][]string{
			bridge.MastodonNetwork: {"m1@mastodon.social", "m2@mastodon.social", "m0@mastodon.social"},
			bridge.ThreadsNetwork:  {"t1@threads.net", "t0@threads.net"},
			bridge.NostrNetwork:    {"npub1n@nostr.com"},
			"bluesky":              {"b1@bsky.social"},
			"":                     {"u1@nonet.example"},
		} {
			users, cur, err := repo.WhoToFollow(network, nil, nil)
			require.NoError(t, err, network)
			require.Equal(t, want, idsOfUsers(users), network)
			require.Equal(t, local_store.EndCursor, cur, network)
			requireOnlyNetwork(t, network, users)
		}

		warp, _, err := repo.WhoToFollow(warpnet.WarpnetName, nil, nil)
		require.NoError(t, err)
		require.Len(t, warp, 2)
		require.Equal(t, native, warp[0].Id, "the tagged native row comes from the warpnet root first")
		requireOnlyNetwork(t, warpnet.WarpnetName, warp)
	})

	t.Run("networks that are not real still never leak", func(t *testing.T) {
		repo, _ := seed(t)
		for _, network := range []string{"None", "none", "Mastodon", "mastodon/", "/", "../", "mastodon//", "\x00", strings.Repeat("x", 4096)} {
			users, cur, err := repo.WhoToFollow(network, nil, nil)
			require.NoError(t, err, network)
			require.Empty(t, users, network)
			require.Equal(t, local_store.EndCursor, cur, network)
		}
	})

	t.Run("a tag named None is found through the None root", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged("None", "x@none.example"))
		users, _, err := repo.WhoToFollow("None", nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"x@none.example"}, idsOfUsers(users))
	})

	t.Run("an own root of offline users hands over to None", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.NostrNetwork, "npub1off@nostr.com"))
		_, err := repo.Update("npub1off@nostr.com", domain.User{IsOffline: true})
		require.NoError(t, err)
		putUsers(t, repo, domain.User{Id: "npub1old@nostr.com", Network: bridge.NostrNetwork})

		users, cur, err := repo.WhoToFollow(bridge.NostrNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"npub1old@nostr.com"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("a row filed under the wrong root never shows up under that root's network", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putRowsIn(t, repo, mastodonNamespace,
			domain.User{Id: "stray@threads.net", Network: bridge.ThreadsNetwork},
			domain.User{Id: "untagged@nonet.example"},
			domain.User{Id: "right@mastodon.social", Network: bridge.MastodonNetwork},
		)
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"right@mastodon.social"}, idsOfUsers(users))
	})

	t.Run("one id stored under two roots does not break the page", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		twin := domain.User{Id: "twin@mastodon.social", Network: bridge.MastodonNetwork}
		putRowsIn(t, repo, mastodonNamespace, twin)
		putUsers(t, repo, twin)
		users, cur, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, local_store.EndCursor, cur)
		requireOnlyNetwork(t, bridge.MastodonNetwork, users)
	})

	t.Run("a corrupt row breaks the page only once it is reached", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "a@mastodon.social"), bridged(bridge.MastodonNetwork, "b@mastodon.social"))
		putRows(t, repo, map[string][]byte{"zz-broken": []byte("{")})

		limit := uint64(2)
		users, next, err := repo.WhoToFollow(bridge.MastodonNetwork, &limit, nil)
		require.NoError(t, err, "the page fills before the None root")
		require.Len(t, users, 2)
		require.Equal(t, noneMarker, next)
		_, _, err = repo.WhoToFollow(bridge.MastodonNetwork, &limit, &next)
		require.Error(t, err)

		own := newWhoToFollowRepo(t)
		txn, err := own.db.NewTxn()
		require.NoError(t, err)
		require.NoError(t, txn.Set(userRowKeyIn(mastodonNamespace, "broken"), []byte("not json")))
		require.NoError(t, txn.Commit())
		_, _, err = own.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.Error(t, err)
	})
}

func TestWhoToFollowCursorsAcrossRoots(t *testing.T) {
	seed := func(t *testing.T) *UserRepo {
		t.Helper()
		repo := newWhoToFollowRepo(t)
		for i := range 3 {
			createAll(t, repo, bridged(bridge.MastodonNetwork, fmt.Sprintf("new%d@mastodon.social", i)))
		}
		createAll(t, repo, bridged(bridge.ThreadsNetwork, "t@threads.net"), bridged(bridge.NostrNetwork, "npub1z@nostr.com"))
		putUsers(t, repo, bulkUsers(bridge.MastodonNetwork, "old", 3)...)
		return repo
	}
	ownKey := func(t *testing.T, repo *UserRepo, id string) string { return storedKeyOf(t, repo, id) }

	t.Run("the end stays the end", func(t *testing.T) {
		end := local_store.EndCursor
		users, cur, err := seed(t).WhoToFollow(bridge.MastodonNetwork, nil, &end)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("nil and empty cursors start from the own root", func(t *testing.T) {
		repo := seed(t)
		empty := ""
		for _, c := range []*string{nil, &empty} {
			users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, c)
			require.NoError(t, err)
			require.Len(t, users, 6)
			require.Equal(t, "new0@mastodon.social", users[0].Id)
		}
	})

	t.Run("the hand-over marker reads only the None rows", func(t *testing.T) {
		marker := noneMarker
		users, cur, err := seed(t).WhoToFollow(bridge.MastodonNetwork, nil, &marker)
		require.NoError(t, err)
		require.Equal(t, []string{"old00000@mastodon.example", "old00001@mastodon.example", "old00002@mastodon.example"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("a key inside the own root resumes there", func(t *testing.T) {
		repo := seed(t)
		c := ownKey(t, repo, "new1@mastodon.social")
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &c)
		require.NoError(t, err)
		require.Equal(t, "new1@mastodon.social", users[0].Id)
		require.Len(t, users, 5)
	})

	t.Run("a key inside None never goes back to the own root", func(t *testing.T) {
		repo := seed(t)
		c := userRowKey("old00001@mastodon.example").String()
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &c)
		require.NoError(t, err)
		require.Equal(t, []string{"old00001@mastodon.example", "old00002@mastodon.example"}, idsOfUsers(users))
	})

	for name, cursor := range map[string]func(*testing.T, *UserRepo) string{
		"another network's row":    func(t *testing.T, r *UserRepo) string { return storedKeyOf(t, r, "t@threads.net") },
		"another network's root":   func(*testing.T, *UserRepo) string { return nostrRoot },
		"a root next to None":      func(*testing.T, *UserRepo) string { return "/USERS/USER/Nonexyz" },
		"the users namespace":      func(*testing.T, *UserRepo) string { return "/USERS/USER/" },
		"the node index":           func(*testing.T, *UserRepo) string { return "/USERS/NODE/gateway" },
		"a fixed key":              func(*testing.T, *UserRepo) string { return "/USERS/USER/None/fixed/new0@mastodon.social" },
		"garbage":                  func(*testing.T, *UserRepo) string { return "garbage" },
		"control bytes":            func(*testing.T, *UserRepo) string { return "\x00\xff" },
		"a double slash":           func(*testing.T, *UserRepo) string { return "//" },
		"the own root without end": func(*testing.T, *UserRepo) string { return "/USERS/USER/mastodon" },
	} {
		t.Run("a forged cursor into "+name+" leaks nothing", func(t *testing.T) {
			repo := seed(t)
			c := cursor(t, repo)
			users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &c)
			require.NoError(t, err)
			requireOnlyNetwork(t, bridge.MastodonNetwork, users)
		})
	}

	for _, limit := range []uint64{1, 2, 3, 4, 5, 6, 7} {
		t.Run(fmt.Sprintf("pages of %d walk both roots once", limit), func(t *testing.T) {
			ids := pageAll(t, seed(t), bridge.MastodonNetwork, limit)
			require.Equal(t, []string{
				"new0@mastodon.social", "new1@mastodon.social", "new2@mastodon.social",
				"old00000@mastodon.example", "old00001@mastodon.example", "old00002@mastodon.example",
			}, ids)
		})
	}

	t.Run("a page that fills at the very end of None ends", func(t *testing.T) {
		limit := uint64(6)
		users, cur, err := seed(t).WhoToFollow(bridge.MastodonNetwork, &limit, nil)
		require.NoError(t, err)
		require.Len(t, users, 6)
		require.Equal(t, local_store.EndCursor, cur)
	})

	t.Run("a full page at the end of the own root hands over once, then ends", func(t *testing.T) {
		repo := seed(t)
		limit := uint64(1)
		users, next, err := repo.WhoToFollow(bridge.NostrNetwork, &limit, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"npub1z@nostr.com"}, idsOfUsers(users))
		require.Equal(t, noneMarker, next)

		rest, end, err := repo.WhoToFollow(bridge.NostrNetwork, &limit, &next)
		require.NoError(t, err)
		require.Empty(t, rest)
		require.Equal(t, local_store.EndCursor, end)
	})
}

func TestWhoToFollowBudgetAcrossRoots(t *testing.T) {
	offline := func(n int) []domain.User {
		users := make([]domain.User, n)
		for i := range users {
			users[i] = domain.User{Id: fmt.Sprintf("gone%05d@mastodon.social", i), Network: bridge.MastodonNetwork, IsOffline: true}
		}
		return users
	}

	t.Run("a budget spent exactly at the end of the own root hands over", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putRowsIn(t, repo, mastodonNamespace, offline(maxRecommendedScan)...)
		putUsers(t, repo, domain.User{Id: "old@mastodon.social", Network: bridge.MastodonNetwork})

		users, next, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, noneMarker, next)

		users, end, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &next)
		require.NoError(t, err)
		require.Equal(t, []string{"old@mastodon.social"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, end)
	})

	t.Run("a budget spent inside the own root resumes inside it", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putRowsIn(t, repo, mastodonNamespace, offline(maxRecommendedScan+5)...)
		putRowsIn(t, repo, mastodonNamespace, domain.User{Id: "zzz@mastodon.social", Network: bridge.MastodonNetwork})
		putUsers(t, repo, domain.User{Id: "old@mastodon.social", Network: bridge.MastodonNetwork})

		users, next, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.True(t, strings.HasPrefix(next, mastodonRoot), next)

		users, end, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &next)
		require.NoError(t, err)
		require.Equal(t, []string{"zzz@mastodon.social", "old@mastodon.social"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, end)
	})

	t.Run("a budget spent inside None resumes inside None", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "own@mastodon.social"))
		putUsers(t, repo, bulkUsers(bridge.ThreadsNetwork, "thr", maxRecommendedScan)...)
		putUsers(t, repo, domain.User{Id: "zzz@mastodon.social", Network: bridge.MastodonNetwork})

		users, next, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"own@mastodon.social"}, idsOfUsers(users))
		require.True(t, strings.HasPrefix(next, noneRoot), next)

		users, end, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, &next)
		require.NoError(t, err)
		require.Equal(t, []string{"zzz@mastodon.social"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, end)
	})

	t.Run("a crowded None does not slow down another network's own root", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers(bridge.ThreadsNetwork, "thr", 2*maxRecommendedScan)...)
		for i := range 25 {
			createAll(t, repo, bridged(bridge.NostrNetwork, fmt.Sprintf("npub1%02d@nostr.com", i)))
		}
		users, next, err := repo.WhoToFollow(bridge.NostrNetwork, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 20)
		require.True(t, strings.HasPrefix(next, nostrRoot), next)
	})
}

func TestRootsUnderConcurrentWritesAndReads(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	networks := []string{warpnet.WarpnetName, bridge.MastodonNetwork, bridge.ThreadsNetwork, bridge.NostrNetwork, "bluesky", ""}
	const perNetwork = 30

	var wg sync.WaitGroup
	for _, network := range networks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perNetwork {
				id := fmt.Sprintf("%s-%02d@c.example", network, i)
				if network == warpnet.WarpnetName {
					id = ulid.Make().String()
				}
				if _, err := repo.Create(domain.User{Id: id, NodeId: "n-" + id, Network: network}); err != nil {
					t.Errorf("create %s: %v", id, err)
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				users, _, err := repo.WhoToFollow(network, nil, nil)
				if err != nil {
					t.Errorf("%q: %v", network, err)
					return
				}
				for _, u := range users {
					if resolveNetwork(u) != network {
						t.Errorf("%s leaked into %q", u.Id, network)
					}
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 10 {
			if _, _, err := repo.List(nil, nil); err != nil {
				t.Errorf("list: %v", err)
			}
		}
	}()
	wg.Wait()

	for _, network := range networks {
		require.Len(t, pageAll(t, repo, network, 7), perNetwork, network)
	}
	limit := uint64(local_store.MaxPageLimit)
	all, _, err := repo.List(&limit, nil)
	require.NoError(t, err)
	require.Len(t, all, perNetwork*len(networks))
}

func TestUpdateThatChangesTheNetwork(t *testing.T) {
	for _, tc := range []struct{ name, from, to string }{
		{"from one bridged network to another", bridge.MastodonNetwork, bridge.ThreadsNetwork},
		{"from a bridged network to Warpnet", bridge.NostrNetwork, warpnet.WarpnetName},
		{"from a bridged network to one without a root", bridge.ThreadsNetwork, "bluesky"},
		{"from no network to a bridged one", "", bridge.MastodonNetwork},
		{"from a network without a root to a bridged one", "bluesky", bridge.NostrNetwork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newWhoToFollowRepo(t)
			createAll(t, repo, domain.User{Id: "mover@x.example", NodeId: "mover-node", Network: tc.from})
			_, err := repo.Update("mover@x.example", domain.User{Network: tc.to})
			require.NoError(t, err)

			users, _, err := repo.WhoToFollow(tc.to, nil, nil)
			require.NoError(t, err)
			require.Equal(t, []string{"mover@x.example"}, idsOfUsers(users), "found under the new network")
			if tc.from != "" {
				users, _, err = repo.WhoToFollow(tc.from, nil, nil)
				require.NoError(t, err)
				require.Empty(t, users, "gone from the old network")
			}
			got, err := repo.GetByNodeID("mover-node")
			require.NoError(t, err)
			require.Equal(t, tc.to, got.Network)
			limit := uint64(10)
			all, _, err := repo.List(&limit, nil)
			require.NoError(t, err)
			require.Equal(t, []string{"mover@x.example"}, idsOfUsers(all), "one row, not two")
		})
	}
}

func TestUpdateMovesARowOnlyWhenItsRootChanges(t *testing.T) {
	t.Run("the old key is gone after a move", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "mover@x.example"))
		before := storedKeyOf(t, repo, "mover@x.example")
		_, err := repo.Update("mover@x.example", domain.User{Network: bridge.ThreadsNetwork, RoundTripTime: 42})
		require.NoError(t, err)

		after := storedKeyOf(t, repo, "mover@x.example")
		require.Equal(t, threadsRoot+"42/mover@x.example", after)
		_, err = repo.db.Get(local_store.DatabaseKey(before))
		require.True(t, local_store.IsNotFoundError(err), "the old row must not linger")
	})

	t.Run("an alias of the same network does not move the row", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		native := ulid.Make().String()
		createAll(t, repo, domain.User{Id: native, NodeId: "n1", Network: "testnet"})
		before := storedKeyOf(t, repo, native)
		_, err := repo.Update(native, domain.User{Network: warpnet.WarpnetName, RoundTripTime: 9})
		require.NoError(t, err)
		require.Equal(t, before, storedKeyOf(t, repo, native))
	})

	t.Run("a None row whose network stays the same is left where it is", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putStoredUnderNone(t, repo, domain.User{Id: "old@mastodon.social", NodeId: "gateway", Network: bridge.MastodonNetwork})
		before := storedKeyOf(t, repo, "old@mastodon.social")
		_, err := repo.Update("old@mastodon.social", domain.User{Bio: "still here"})
		require.NoError(t, err)
		require.Equal(t, before, storedKeyOf(t, repo, "old@mastodon.social"))
	})

	t.Run("a None row whose network changes moves to the new root", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putStoredUnderNone(t, repo, domain.User{Id: "old@x.example", Network: bridge.MastodonNetwork})
		_, err := repo.Update("old@x.example", domain.User{Network: bridge.NostrNetwork})
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(storedKeyOf(t, repo, "old@x.example"), nostrRoot))

		users, _, err := repo.WhoToFollow(bridge.NostrNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"old@x.example"}, idsOfUsers(users))
	})

	t.Run("a move without a node id writes no node index", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, domain.User{Id: "nodeless@x.example", Network: bridge.MastodonNetwork})
		_, err := repo.Update("nodeless@x.example", domain.User{Network: bridge.ThreadsNetwork})
		require.NoError(t, err)
		_, err = repo.GetByNodeID("")
		require.ErrorIs(t, err, ErrUserNotFound)
		got, err := repo.Get("nodeless@x.example")
		require.NoError(t, err)
		require.Equal(t, bridge.ThreadsNetwork, got.Network)
	})

	t.Run("moving back and forth leaves one row", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		createAll(t, repo, bridged(bridge.MastodonNetwork, "pingpong@x.example"))
		for _, network := range []string{bridge.ThreadsNetwork, bridge.NostrNetwork, "", bridge.MastodonNetwork, "bluesky", warpnet.WarpnetName, bridge.MastodonNetwork} {
			_, err := repo.Update("pingpong@x.example", domain.User{Network: network})
			require.NoError(t, err)
		}
		limit := uint64(10)
		all, _, err := repo.List(&limit, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"pingpong@x.example"}, idsOfUsers(all))
		users, _, err := repo.WhoToFollow(bridge.MastodonNetwork, nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"pingpong@x.example"}, idsOfUsers(users))
		got, err := repo.GetByNodeID("gateway")
		require.NoError(t, err)
		require.Equal(t, "pingpong@x.example", got.Id)
	})

	t.Run("an update of a user that does not exist moves nothing", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		_, err := repo.Update("ghost@x.example", domain.User{Network: bridge.NostrNetwork})
		require.Error(t, err)
		limit := uint64(10)
		all, _, err := repo.List(&limit, nil)
		require.NoError(t, err)
		require.Empty(t, all)
	})
}
