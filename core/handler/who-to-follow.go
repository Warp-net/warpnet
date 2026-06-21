package handler

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Warp-net/warpnet/core/mastodon"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	whoToFollowDefaultLimit   = 20
	whoToFollowMaxLimit       = 100
	whoToFollowCandidateScan  = 500
	whoToFollowSnapshotTTL    = 10 * time.Minute
	whoToFollowFanoutCap      = 30
	whoToFollowFanoutLimit    = 50
	whoToFollowFollowingsPage = 200
)

// WhoToFollowStreamer lets the recommender pull a remote user's followings
// from that user's home node. The social graph is split across nodes: a node
// only stores its own followings/followers, so followings-of-followings needs
// a remote call per followee.
type WhoToFollowStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) ([]byte, error)
}

// whoToFollowSnapshot is the cached, ranked recommendation list for the owner.
type whoToFollowSnapshot struct {
	users       []domain.User
	builtAt     time.Time
	hasFoF      bool
	followCount uint64
}

// whoToFollowRecommender builds owner-centric recommendations. A request is
// served from the snapshot immediately (RTT-ranked baseline on first call);
// the heavier followings-of-followings pass runs in the background and updates
// the snapshot for the next open.
type whoToFollowRecommender struct {
	authRepo   UserAuthStorer
	userRepo   UserFetcher
	followRepo UserFollowsCounter
	streamer   WhoToFollowStreamer

	mu       sync.Mutex
	snapshot *whoToFollowSnapshot
	building bool
}

func StreamGetWhoToFollowHandler(
	authRepo UserAuthStorer,
	userRepo UserFetcher,
	followRepo UserFollowsCounter,
	streamer WhoToFollowStreamer,
) warpnet.WarpHandlerFunc {
	rec := &whoToFollowRecommender{
		authRepo:   authRepo,
		userRepo:   userRepo,
		followRepo: followRepo,
		streamer:   streamer,
	}
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetAllUsersEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}

		if ev.UserId == "" {
			return nil, errEmptyUserId
		}

		users, err := rec.recommendations()
		if err != nil {
			return nil, err
		}
		return paginateUsers(users, ev.Cursor, ev.Limit), nil
	}
}

// paginateUsers serves a page out of the ranked list using an offset cursor
// (stringified next index; empty when exhausted).
func paginateUsers(users []domain.User, cursor *string, limit *uint64) event.UsersResponse {
	offset := 0
	if cursor != nil {
		if n, err := strconv.Atoi(*cursor); err == nil && n > 0 {
			offset = n
		}
	}
	size := whoToFollowDefaultLimit
	if limit != nil && *limit > 0 {
		size = int(min(*limit, whoToFollowMaxLimit)) //nolint:gosec // bounded by whoToFollowMaxLimit
	}
	if offset >= len(users) {
		return event.UsersResponse{Users: []domain.User{}}
	}
	end := min(offset+size, len(users))
	next := ""
	if end < len(users) {
		next = strconv.Itoa(end)
	}
	return event.UsersResponse{Cursor: next, Users: users[offset:end]}
}

func (r *whoToFollowRecommender) recommendations() ([]domain.User, error) {
	owner := r.authRepo.GetOwner()
	followCount, err := r.followRepo.GetFollowingsCount(owner.UserId)
	if err != nil {
		log.Errorf("who to follow: followings count: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.snapshot == nil {
		users, buildErr := r.buildBaseline(owner)
		if buildErr != nil {
			return nil, buildErr
		}
		r.snapshot = &whoToFollowSnapshot{users: users, builtAt: time.Now(), followCount: followCount}
	}

	snap := r.snapshot
	stale := !snap.hasFoF || snap.followCount != followCount || time.Since(snap.builtAt) > whoToFollowSnapshotTTL
	if stale && !r.building {
		r.building = true
		go r.refresh()
	}
	return snap.users, nil
}

// buildBaseline is the fast, fully local path: RTT-nearest candidates with the
// Mastodon entry pinned. It is returned synchronously so the first request
// never blocks on the network.
func (r *whoToFollowRecommender) buildBaseline(owner domain.Owner) ([]domain.User, error) {
	scan := uint64(whoToFollowCandidateScan)
	candidates, _, err := r.userRepo.WhoToFollow(&scan, nil)
	if err != nil {
		return nil, err
	}
	followed := r.followedSet(owner.UserId)
	sortByRTT(candidates)
	return r.assemble(owner, followed, candidates), nil
}

// refresh recomputes the snapshot in the background. For an established account
// it ranks followings-of-followings first and fills the tail by RTT; for a new
// account (empty social graph) it stays RTT-only.
func (r *whoToFollowRecommender) refresh() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("who to follow: refresh: recovered: %v", rec)
		}
		r.mu.Lock()
		r.building = false
		r.mu.Unlock()
	}()

	owner := r.authRepo.GetOwner()
	followed := r.followedSet(owner.UserId)

	scan := uint64(whoToFollowCandidateScan)
	candidates, _, err := r.userRepo.WhoToFollow(&scan, nil)
	if err != nil {
		log.Errorf("who to follow: refresh: candidates: %v", err)
		return
	}
	sortByRTT(candidates)

	var ordered []domain.User
	if len(followed) == 0 {
		ordered = candidates
	} else {
		ordered = append(r.followingsOfFollowings(owner, followed), candidates...)
	}

	final := r.assemble(owner, followed, ordered)
	followCount, _ := r.followRepo.GetFollowingsCount(owner.UserId)

	r.mu.Lock()
	r.snapshot = &whoToFollowSnapshot{users: final, builtAt: time.Now(), hasFoF: true, followCount: followCount}
	r.mu.Unlock()
}

// followedSet returns every account the owner follows (full set, paginated),
// used both to exclude already-followed users and to seed the FoF pass.
func (r *whoToFollowRecommender) followedSet(ownerId string) map[string]struct{} {
	followed := map[string]struct{}{}
	page := uint64(whoToFollowFollowingsPage)
	var cursor *string
	for {
		ids, cur, err := r.followRepo.GetFollowings(ownerId, &page, cursor)
		if err != nil {
			log.Errorf("who to follow: get followings: %v", err)
			break
		}
		for _, id := range ids {
			followed[id] = struct{}{}
		}
		if cur == "" || uint64(len(ids)) < page {
			break
		}
		c := cur
		cursor = &c
	}
	return followed
}

// followingsOfFollowings fans out to each followee's home node, asking for the
// accounts they follow, and ranks the union by how many of the owner's
// followings follow them (frequency), tie-broken by RTT.
func (r *whoToFollowRecommender) followingsOfFollowings(owner domain.Owner, followed map[string]struct{}) []domain.User {
	freq := map[string]int{}
	queried := 0
	for followeeId := range followed {
		if queried >= whoToFollowFanoutCap {
			break
		}
		followee, err := r.userRepo.Get(followeeId)
		if err != nil || followee.NodeId == "" || followee.NodeId == owner.NodeId {
			continue // unknown locally or not routable to a remote node
		}
		queried++

		lim := uint64(whoToFollowFanoutLimit)
		resp, err := r.streamer.GenericStream(
			followee.NodeId,
			event.PUBLIC_GET_FOLLOWINGS,
			event.GetFollowingsEvent{UserId: followeeId, Limit: &lim},
		)
		if err != nil {
			log.Debugf("who to follow: fanout %s: %v", followee.NodeId, err)
			continue
		}
		var fr event.FollowingsResponse
		if err := json.Unmarshal(resp, &fr); err != nil {
			continue
		}
		for _, id := range fr.Followings {
			sid := string(id)
			if sid == "" || sid == owner.UserId {
				continue
			}
			if _, ok := followed[sid]; ok {
				continue // already following
			}
			freq[sid]++
		}
	}

	scored := make([]domain.User, 0, len(freq))
	counts := make(map[string]int, len(freq))
	for id, c := range freq {
		u, err := r.userRepo.Get(id)
		if err != nil {
			continue // can only recommend accounts we can resolve locally
		}
		scored = append(scored, u)
		counts[u.Id] = c
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if counts[scored[i].Id] != counts[scored[j].Id] {
			return counts[scored[i].Id] > counts[scored[j].Id]
		}
		return scored[i].RoundTripTime < scored[j].RoundTripTime
	})
	return scored
}

func sortByRTT(users []domain.User) {
	sort.SliceStable(users, func(i, j int) bool {
		return users[i].RoundTripTime < users[j].RoundTripTime
	})
}

// assemble applies the structural exclusions (self, already-followed, one user
// per node) and pins the Mastodon entry account on top until it is followed.
func (r *whoToFollowRecommender) assemble(owner domain.Owner, followed map[string]struct{}, ordered []domain.User) []domain.User {
	out := make([]domain.User, 0, len(ordered)+1)
	seenUser := map[string]struct{}{}
	seenNode := map[string]struct{}{}

	if entry, err := r.userRepo.Get(mastodon.EntryHandle); err == nil && entry.Id != "" {
		if _, done := followed[entry.Id]; !done && entry.Id != owner.UserId {
			out = append(out, entry)
			seenUser[entry.Id] = struct{}{}
		}
	}

	for _, u := range ordered {
		if u.Id == "" || u.IsOffline {
			continue
		}
		if u.Id == owner.UserId || u.NodeId == owner.NodeId {
			continue
		}
		if _, ok := followed[u.Id]; ok {
			continue
		}
		if _, ok := seenUser[u.Id]; ok {
			continue
		}
		if u.NodeId != "" && u.Network != mastodon.Network {
			if _, ok := seenNode[u.NodeId]; ok {
				continue
			}
			seenNode[u.NodeId] = struct{}{}
		}
		seenUser[u.Id] = struct{}{}
		out = append(out, u)
	}
	return out
}
