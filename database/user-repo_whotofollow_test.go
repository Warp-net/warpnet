//nolint:all
package database

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func userRowKey(id string) local_store.DatabaseKey {
	return local_store.NewPrefixBuilder(UsersRepoName).
		AddSubPrefix(userSubNamespace).
		AddRootID("None").
		AddRange(local_store.RangePrefix("500")).
		AddParentId(id).
		Build()
}

// putRows writes user rows straight into the scanned keyspace: thousands of
// them through Create would make the scan-budget tests crawl under -race.
func putRows(t *testing.T, repo *UserRepo, rows map[string][]byte) {
	t.Helper()
	txn, err := repo.db.NewTxn()
	require.NoError(t, err)
	defer txn.Rollback()
	for id, value := range rows {
		require.NoError(t, txn.Set(userRowKey(id), value))
	}
	require.NoError(t, txn.Commit())
}

func putUsers(t *testing.T, repo *UserRepo, users ...domain.User) {
	t.Helper()
	rows := make(map[string][]byte, len(users))
	for _, u := range users {
		data, err := json.Marshal(u)
		require.NoError(t, err)
		rows[u.Id] = data
	}
	putRows(t, repo, rows)
}

func bulkUsers(network, idPrefix string, n int) []domain.User {
	users := make([]domain.User, n)
	for i := range users {
		users[i] = domain.User{Id: fmt.Sprintf("%s%05d@%s.example", idPrefix, i, network), NodeId: "gateway", Network: network}
	}
	return users
}

// pageAll walks a network to its end and fails on a repeat or a runaway cursor.
func pageAll(t *testing.T, repo *UserRepo, network string, limit uint64) []string {
	t.Helper()
	var ids []string
	seen := map[string]struct{}{}
	cursor := ""
	for range 10_000 {
		users, next, err := repo.WhoToFollow(network, &limit, &cursor)
		require.NoError(t, err)
		for _, u := range users {
			require.Equal(t, network, resolveNetwork(u), u.Id)
			require.NotContains(t, seen, u.Id, "returned twice")
			seen[u.Id] = struct{}{}
			ids = append(ids, u.Id)
		}
		if next == local_store.EndCursor {
			return ids
		}
		require.NotEmpty(t, next, "a page that is not the last must say where to go on")
		cursor = next
	}
	t.Fatal("paging never ended")
	return nil
}

func TestResolveNetwork(t *testing.T) {
	native := ulid.Make().String()
	cases := []struct {
		name string
		user domain.User
		want string
	}{
		{"untagged ULID", domain.User{Id: native}, warpnet.WarpnetName},
		{"untagged handle", domain.User{Id: "bob@legacy.example"}, ""},
		{"untagged empty id", domain.User{}, ""},
		{"untagged ULID one char short", domain.User{Id: native[:25]}, ""},
		{"untagged ULID one char long", domain.User{Id: native + "0"}, ""},
		{"untagged ULID with a letter Crockford forbids", domain.User{Id: "U" + native[1:]}, ""},
		{"untagged ULID that overflows 128 bits", domain.User{Id: "8" + native[1:]}, ""},
		{"warpnet tag on a handle", domain.User{Id: "bob@legacy.example", Network: warpnet.WarpnetName}, warpnet.WarpnetName},
		{"testnet tag", domain.User{Id: "x", Network: "testnet"}, warpnet.WarpnetName},
		{"mainnet tag", domain.User{Id: "x", Network: "mainnet"}, warpnet.WarpnetName},
		{"bridged tag beats a ULID id", domain.User{Id: native, Network: "mastodon"}, "mastodon"},
		{"a network nobody knows yet", domain.User{Id: "x@y", Network: "bluesky"}, "bluesky"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, resolveNetwork(tc.user))
		})
	}
}

func TestListNetworksEdges(t *testing.T) {
	t.Run("an empty store lists nothing", func(t *testing.T) {
		networks, err := newWhoToFollowRepo(t).ListNetworks()
		require.NoError(t, err)
		require.Empty(t, networks)
	})

	t.Run("a network whose every account is offline is not listed", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo,
			domain.User{Id: "a@m.example", Network: "mastodon", IsOffline: true},
			domain.User{Id: "b@m.example", Network: "mastodon", IsOffline: true},
			domain.User{Id: "c@t.example", Network: "threads"},
		)
		networks, err := repo.ListNetworks()
		require.NoError(t, err)
		require.Equal(t, []string{"threads"}, networks)
	})

	t.Run("the order is the keyspace order and does not wobble", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo,
			domain.User{Id: "c@t.example", Network: "threads"},
			domain.User{Id: "a@m.example", Network: "mastodon"},
			domain.User{Id: "b@n.example", Network: "nostr"},
			domain.User{Id: "d@m.example", Network: "mastodon"},
		)
		first, err := repo.ListNetworks()
		require.NoError(t, err)
		require.Equal(t, []string{"mastodon", "nostr", "threads"}, first)
		for range 20 {
			again, err := repo.ListNetworks()
			require.NoError(t, err)
			require.Equal(t, first, again)
		}
	})

	t.Run("a network on the second scan page is listed", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", int(recommendedScanPage)+50)...)
		putUsers(t, repo, domain.User{Id: "zzz@t.example", Network: "threads"})
		networks, err := repo.ListNetworks()
		require.NoError(t, err)
		require.Equal(t, []string{"mastodon", "threads"}, networks)
	})

	t.Run("a network buried past the scan budget stays unlisted", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", maxRecommendedScan)...)
		putUsers(t, repo, domain.User{Id: "zzz@t.example", Network: "threads"})
		networks, err := repo.ListNetworks()
		require.NoError(t, err)
		require.Equal(t, []string{"mastodon"}, networks)
	})

	t.Run("a corrupt row fails the listing", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, domain.User{Id: "a@m.example", Network: "mastodon"})
		putRows(t, repo, map[string][]byte{"broken": []byte(`{"id":`)})
		_, err := repo.ListNetworks()
		require.Error(t, err)
	})

	t.Run("node and fixed keys never pass for users", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		_, err := repo.Create(domain.User{Id: "a@m.example", NodeId: "gateway", Network: "mastodon"})
		require.NoError(t, err)
		networks, err := repo.ListNetworks()
		require.NoError(t, err)
		require.Equal(t, []string{"mastodon"}, networks)
	})
}

func TestWhoToFollowLimits(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	putUsers(t, repo, bulkUsers("mastodon", "aaa", int(local_store.MaxPageLimit)+5)...)

	for name, limit := range map[string]*uint64{"no limit": nil, "a zero limit": new(uint64)} {
		t.Run(name+" means 20", func(t *testing.T) {
			users, next, err := repo.WhoToFollow("mastodon", limit, nil)
			require.NoError(t, err)
			require.Len(t, users, 20)
			require.NotEqual(t, local_store.EndCursor, next)
		})
	}

	t.Run("a huge limit is capped at the store page limit", func(t *testing.T) {
		huge := uint64(1 << 62)
		users, next, err := repo.WhoToFollow("mastodon", &huge, nil)
		require.NoError(t, err)
		require.Len(t, users, int(local_store.MaxPageLimit))
		require.NotEqual(t, local_store.EndCursor, next)

		rest, end, err := repo.WhoToFollow("mastodon", &huge, &next)
		require.NoError(t, err)
		require.Len(t, rest, 5)
		require.Equal(t, local_store.EndCursor, end)
		require.NotEqual(t, users[len(users)-1].Id, rest[0].Id)
	})

	t.Run("a limit of one walks every account exactly once", func(t *testing.T) {
		small := newWhoToFollowRepo(t)
		putUsers(t, small, bulkUsers("mastodon", "aaa", 7)...)
		putUsers(t, small, bulkUsers("threads", "bbb", 3)...)
		require.Len(t, pageAll(t, small, "mastodon", 1), 7)
		require.Len(t, pageAll(t, small, "threads", 1), 3)
	})
}

func TestWhoToFollowCursorEdges(t *testing.T) {
	t.Run("a page that fills exactly at the end leads to an empty last page", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", 3)...)
		limit := uint64(3)
		users, next, err := repo.WhoToFollow("mastodon", &limit, nil)
		require.NoError(t, err)
		require.Len(t, users, 3)
		rest, end, err := repo.WhoToFollow("mastodon", &limit, &next)
		require.NoError(t, err)
		require.Empty(t, rest)
		require.Equal(t, local_store.EndCursor, end)
	})

	t.Run("the end cursor stays the end", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", 3)...)
		end := local_store.EndCursor
		users, next, err := repo.WhoToFollow("mastodon", nil, &end)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, local_store.EndCursor, next)
	})

	t.Run("a network nobody is on ends at once", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", 3)...)
		users, next, err := repo.WhoToFollow("bluesky", nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, local_store.EndCursor, next)
	})

	t.Run("an empty store ends at once", func(t *testing.T) {
		users, next, err := newWhoToFollowRepo(t).WhoToFollow(warpnet.WarpnetName, nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.Equal(t, local_store.EndCursor, next)
	})

	for name, cursor := range map[string]string{
		"garbage":                     "definitely not a key",
		"the keyspace root":           "/",
		"the node index":              "/USERS/NODE/gateway",
		"another repo":                "/TWEETS/TWEET/None",
		"a fixed key":                 "/USERS/USER/None/FIXED/aaa00000@mastodon.example",
		"a key past the users prefix": "/USERS/USER/Nonf",
		"control bytes":               "\x00\xff\x00",
		"a very long key":             "/USERS/USER/None/" + strings.Repeat("z", 64<<10),
	} {
		t.Run("a forged cursor into "+name+" leaks nothing", func(t *testing.T) {
			repo := newWhoToFollowRepo(t)
			putUsers(t, repo, bulkUsers("mastodon", "aaa", 5)...)
			putUsers(t, repo, bulkUsers("threads", "bbb", 5)...)
			c := cursor
			users, _, err := repo.WhoToFollow("mastodon", nil, &c)
			require.NoError(t, err)
			for _, u := range users {
				require.Equal(t, "mastodon", u.Network)
			}
		})
	}

	t.Run("a cursor on another network's row resumes from there", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo,
			domain.User{Id: "a1@m.example", Network: "mastodon"},
			domain.User{Id: "b1@t.example", Network: "threads"},
			domain.User{Id: "c1@m.example", Network: "mastodon"},
		)
		c := userRowKey("b1@t.example").String()
		users, next, err := repo.WhoToFollow("mastodon", nil, &c)
		require.NoError(t, err)
		require.Equal(t, []string{"c1@m.example"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, next)
	})
}

func idsOfUsers(users []domain.User) []string {
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	return ids
}

func TestWhoToFollowScanBudget(t *testing.T) {
	t.Run("an account on the second scan page is found in one call", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", int(recommendedScanPage)+1)...)
		putUsers(t, repo, domain.User{Id: "zzz@t.example", Network: "threads"})
		users, next, err := repo.WhoToFollow("threads", nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"zzz@t.example"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, next)
	})

	t.Run("the last row the budget reaches is still found", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", maxRecommendedScan-1)...)
		putUsers(t, repo, domain.User{Id: "zzz@t.example", Network: "threads"})
		users, _, err := repo.WhoToFollow("threads", nil, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"zzz@t.example"}, idsOfUsers(users))
	})

	t.Run("a spent budget hands back a cursor instead of a false end", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", maxRecommendedScan)...)
		putUsers(t, repo, domain.User{Id: "zzz@t.example", Network: "threads"})

		users, next, err := repo.WhoToFollow("threads", nil, nil)
		require.NoError(t, err)
		require.Empty(t, users)
		require.NotEmpty(t, next)
		require.NotEqual(t, local_store.EndCursor, next, "the threads account is still out there")

		users, end, err := repo.WhoToFollow("threads", nil, &next)
		require.NoError(t, err)
		require.Equal(t, []string{"zzz@t.example"}, idsOfUsers(users))
		require.Equal(t, local_store.EndCursor, end)
	})

	t.Run("a buried network is still reachable to the end, page by page", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", 2*maxRecommendedScan+10)...)
		putUsers(t, repo, bulkUsers("threads", "zzz", 3)...)
		require.Len(t, pageAll(t, repo, "threads", 20), 3)
	})
}

func TestWhoToFollowCorruptRows(t *testing.T) {
	for name, value := range map[string][]byte{
		"truncated json": []byte(`{"id":"x","network":`),
		"not json":       []byte("\x00\x01binary"),
		"a json array":   []byte(`["x"]`),
		"a wrong type":   []byte(`{"id":42}`),
	} {
		t.Run(name+" fails the page of any network", func(t *testing.T) {
			repo := newWhoToFollowRepo(t)
			putUsers(t, repo, domain.User{Id: "a@m.example", Network: "mastodon"})
			putRows(t, repo, map[string][]byte{"broken": value})
			for _, network := range []string{"mastodon", "threads", warpnet.WarpnetName} {
				_, _, err := repo.WhoToFollow(network, nil, nil)
				require.Error(t, err, network)
			}
		})
	}

	t.Run("a corrupt row past the filled page is never read", func(t *testing.T) {
		repo := newWhoToFollowRepo(t)
		putUsers(t, repo, bulkUsers("mastodon", "aaa", 3)...)
		putRows(t, repo, map[string][]byte{"zzz-broken": []byte("{")})
		limit := uint64(2)
		users, next, err := repo.WhoToFollow("mastodon", &limit, nil)
		require.NoError(t, err)
		require.Len(t, users, 2)
		_, _, err = repo.WhoToFollow("mastodon", &limit, &next)
		require.Error(t, err, "the next page runs into it")
	})
}

func TestWhoToFollowWhileTheStoreChanges(t *testing.T) {
	seed := func(t *testing.T) (*UserRepo, string) {
		t.Helper()
		repo := newWhoToFollowRepo(t)
		seedNetwork(t, repo, "mastodon", "aaa", "mastodon.social", 5)
		limit := uint64(2)
		users, next, err := repo.WhoToFollow("mastodon", &limit, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"aaa00@mastodon.social", "aaa01@mastodon.social"}, idsOfUsers(users))
		return repo, next
	}
	rest := func(t *testing.T, repo *UserRepo, cursor string) []string {
		t.Helper()
		limit := uint64(10)
		users, end, err := repo.WhoToFollow("mastodon", &limit, &cursor)
		require.NoError(t, err)
		require.Equal(t, local_store.EndCursor, end)
		return idsOfUsers(users)
	}

	t.Run("the account the cursor points at is deleted", func(t *testing.T) {
		repo, cursor := seed(t)
		require.NoError(t, repo.Delete("aaa02@mastodon.social"))
		require.Equal(t, []string{"aaa03@mastodon.social", "aaa04@mastodon.social"}, rest(t, repo, cursor))
	})

	t.Run("the account the cursor points at goes offline", func(t *testing.T) {
		repo, cursor := seed(t)
		_, err := repo.Update("aaa02@mastodon.social", domain.User{IsOffline: true})
		require.NoError(t, err)
		require.Equal(t, []string{"aaa03@mastodon.social", "aaa04@mastodon.social"}, rest(t, repo, cursor))
	})

	t.Run("every account left goes offline", func(t *testing.T) {
		repo, cursor := seed(t)
		for _, id := range []string{"aaa02", "aaa03", "aaa04"} {
			_, err := repo.Update(id+"@mastodon.social", domain.User{IsOffline: true})
			require.NoError(t, err)
		}
		require.Empty(t, rest(t, repo, cursor))
	})

	t.Run("accounts arrive on both sides of the cursor", func(t *testing.T) {
		repo, cursor := seed(t)
		for _, id := range []string{"aaa00b@mastodon.social", "aaa03b@mastodon.social"} {
			_, err := repo.Create(domain.User{Id: id, NodeId: "gateway", Network: "mastodon"})
			require.NoError(t, err)
		}
		require.Equal(t,
			[]string{"aaa02@mastodon.social", "aaa03@mastodon.social", "aaa03b@mastodon.social", "aaa04@mastodon.social"},
			rest(t, repo, cursor))
	})

	t.Run("an account changes network between pages", func(t *testing.T) {
		repo, cursor := seed(t)
		require.NoError(t, repo.Delete("aaa03@mastodon.social"))
		_, err := repo.Create(domain.User{Id: "aaa03@mastodon.social", NodeId: "gateway", Network: "threads"})
		require.NoError(t, err)
		require.Equal(t, []string{"aaa02@mastodon.social", "aaa04@mastodon.social"}, rest(t, repo, cursor))
	})
}

func TestWhoToFollowConcurrentReaders(t *testing.T) {
	repo := newWhoToFollowRepo(t)
	want := map[string]int{"mastodon": 37, "threads": 11, "nostr": 23, warpnet.WarpnetName: 0}
	for network, n := range want {
		putUsers(t, repo, bulkUsers(network, network[:3], n)...)
	}
	for i := range 9 {
		putUsers(t, repo, domain.User{Id: ulid.Make().String(), NodeId: fmt.Sprintf("node-%d", i)})
	}
	want[warpnet.WarpnetName] = 9

	var wg sync.WaitGroup
	for network, n := range want {
		for reader := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				limit := uint64(3 + reader)
				cursor := ""
				got := 0
				for range 1000 {
					users, next, err := repo.WhoToFollow(network, &limit, &cursor)
					if err != nil {
						t.Errorf("%s: %v", network, err)
						return
					}
					got += len(users)
					if next == local_store.EndCursor {
						break
					}
					cursor = next
				}
				if got != n {
					t.Errorf("%s reader %d: got %d, want %d", network, reader, got, n)
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.ListNetworks(); err != nil {
				t.Errorf("list networks: %v", err)
			}
		}()
	}
	wg.Wait()
}
