// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package rating

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/core/crdt/broadcast"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/json"
	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	replicationTimeout = 30 * time.Second
	replicationTick    = 25 * time.Millisecond

	observedPeer = "12D3KooWD3eckifWpRn9wQpMG9R9hX3sD158z7EqHWmweQAJU5SA"
	otherPeer    = "12D3KooWJWoaqZhDaoEFshF7Rh1bpY9ohihFhzcW6d69Lr2NASuq"
)

// gossipAdapter mirrors core/pubsub.Gossip: a topic is joined and subscribed
// once, a later SubscribeRaw only swaps the handler, and a node's own
// messages reach its handler like everyone else's.
type gossipAdapter struct {
	ctx context.Context
	ps  *pubsub.PubSub

	mu       sync.Mutex
	topics   map[string]*pubsub.Topic
	handlers map[string]func([]byte) error
}

func newGossipAdapter(ctx context.Context, ps *pubsub.PubSub) *gossipAdapter {
	return &gossipAdapter{
		ctx:      ctx,
		ps:       ps,
		topics:   map[string]*pubsub.Topic{},
		handlers: map[string]func([]byte) error{},
	}
}

func (g *gossipAdapter) joinLocked(name string) (*pubsub.Topic, error) {
	if topic, ok := g.topics[name]; ok {
		return topic, nil
	}
	topic, err := g.ps.Join(name)
	if err != nil {
		return nil, err
	}
	g.topics[name] = topic
	return topic, nil
}

func (g *gossipAdapter) SubscribeRaw(name string, h func([]byte) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	topic, err := g.joinLocked(name)
	if err != nil {
		return err
	}
	if _, subscribed := g.handlers[name]; subscribed {
		g.handlers[name] = h
		return nil
	}
	if _, err := topic.Relay(); err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return err
	}
	g.handlers[name] = h
	go g.listen(name, sub)
	return nil
}

func (g *gossipAdapter) listen(name string, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(g.ctx)
		if err != nil {
			return
		}
		g.mu.Lock()
		h := g.handlers[name]
		g.mu.Unlock()
		_ = h(msg.Data)
	}
}

func (g *gossipAdapter) PublishRaw(name string, data []byte) error {
	g.mu.Lock()
	topic, err := g.joinLocked(name)
	g.mu.Unlock()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := topic.Publish(ctx, data); err != nil && !errors.Is(err, pubsub.ErrTopicClosed) {
		return err
	}
	return nil
}

// meshTracer mirrors the gossipsub mesh. A topic peer outside it misses a
// publish, and IHAVE gossip skips it once the next heartbeat grafts it.
type meshTracer struct {
	mu   sync.Mutex
	mesh map[string]map[peer.ID]bool
}

func (m *meshTracer) Graft(p peer.ID, topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mesh == nil {
		m.mesh = map[string]map[peer.ID]bool{}
	}
	if m.mesh[topic] == nil {
		m.mesh[topic] = map[peer.ID]bool{}
	}
	m.mesh[topic][p] = true
}

func (m *meshTracer) Prune(p peer.ID, topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mesh[topic], p)
}

func (m *meshTracer) OnClosedOutboundStream(p peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, peers := range m.mesh {
		delete(peers, p)
	}
}

func (m *meshTracer) has(topic string, p peer.ID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mesh[topic][p]
}

func (*meshTracer) OnNewOutboundStream(peer.ID, protocol.ID) {}
func (*meshTracer) Join(string)                              {}
func (*meshTracer) Leave(string)                             {}
func (*meshTracer) ValidateMessage(*pubsub.Message)          {}
func (*meshTracer) DeliverMessage(*pubsub.Message)           {}
func (*meshTracer) RejectMessage(*pubsub.Message, string)    {}
func (*meshTracer) DuplicateMessage(*pubsub.Message)         {}
func (*meshTracer) ThrottlePeer(peer.ID)                     {}
func (*meshTracer) RecvRPC(*pubsub.RPC)                      {}
func (*meshTracer) SendRPC(*pubsub.RPC, peer.ID)             {}
func (*meshTracer) DropRPC(*pubsub.RPC, peer.ID)             {}
func (*meshTracer) UndeliverableMessage(*pubsub.Message)     {}

// replica is one node: a host with its gossipsub, a datastore that outlives
// restarts, and the rating store wired over them as the member node does.
type replica struct {
	name string
	data *dssync.MutexDatastore

	host   host.Host
	ctx    context.Context
	cancel context.CancelFunc
	gossip *gossipAdapter
	mesh   *meshTracer

	store   *Store
	puts    *recordLog
	deletes *recordLog
}

func newReplica(t *testing.T, name string) *replica {
	t.Helper()
	r := &replica{name: name, data: dssync.MutexWrap(datastore.NewMapDatastore())}
	r.boot(t)
	t.Cleanup(r.stop)
	return r
}

// boot brings up the host and its gossipsub, as a node process does on start.
func (r *replica) boot(t *testing.T, opts ...libp2p.Option) {
	t.Helper()
	h, err := libp2p.New(append([]libp2p.Option{libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0")}, opts...)...)
	require.NoError(t, err)
	r.host = h
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.mesh = &meshTracer{}
	ps, err := pubsub.NewGossipSub(r.ctx, h, pubsub.WithRawTracer(r.mesh))
	require.NoError(t, err)
	r.gossip = newGossipAdapter(r.ctx, ps)
}

// open starts a store over the replica's datastore with fresh hook logs.
func (r *replica) open(t *testing.T) {
	t.Helper()
	bc, err := broadcast.NewGossip(r.ctx, r.gossip, GossipTopic)
	require.NoError(t, err)
	s, err := New(r.ctx, bc, r.data, r.host, idleRouter{}, acceptRecords)
	require.NoError(t, err)
	r.puts, r.deletes = &recordLog{}, &recordLog{}
	s.OnPut(r.puts.hook)
	s.OnDelete(r.deletes.hook)
	r.store = s
}

func (r *replica) closeStore(t *testing.T) {
	t.Helper()
	require.NoError(t, r.store.Close())
	r.store = nil
}

// stop tears the replica down in the node's order: store, pubsub, host.
func (r *replica) stop() {
	if r.store != nil {
		_ = r.store.Close()
		r.store = nil
	}
	r.cancel()
	_ = r.host.Close()
}

func (r *replica) id() string { return r.host.ID().String() }

// eventuallyLists waits until r lists exactly want about peerID.
func (r *replica) eventuallyLists(t *testing.T, peerID string, want ...domain.RatingRecord) {
	t.Helper()
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		got, err := r.store.List(peerID)
		assert.NoError(collect, err)
		assert.ElementsMatch(collect, want, got)
	}, replicationTimeout, replicationTick, "%s never converged on %s", r.name, peerID)
}

func (r *replica) eventuallyHooked(t *testing.T, log *recordLog, rec domain.RatingRecord) {
	t.Helper()
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		assert.Contains(collect, log.snapshot(), rec)
	}, replicationTimeout, replicationTick, "%s hooks never saw %+v", r.name, rec)
}

func connect(t *testing.T, a, b *replica) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, a.host.Connect(ctx, peer.AddrInfo{ID: b.host.ID(), Addrs: b.host.Addrs()}))
}

func connectAll(t *testing.T, rs ...*replica) {
	t.Helper()
	for i := range rs {
		for _, other := range rs[i+1:] {
			connect(t, rs[i], other)
		}
	}
}

// awaitMesh waits until every replica has grafted every other one on the
// rating topic. ListPeers is not enough: it only knows who subscribed.
func awaitMesh(t *testing.T, rs ...*replica) {
	t.Helper()
	for _, r := range rs {
		for _, other := range rs {
			if other == r {
				continue
			}
			require.Eventually(t, func() bool {
				return r.mesh.has(GossipTopic, other.host.ID())
			}, replicationTimeout, replicationTick, "%s never grafted %s on the rating topic", r.name, other.name)
		}
	}
}

// settle waits until every replica holds the same single head per DAG: the
// history so far is one chain per day, not branches still being merged.
func settle(t *testing.T, rs ...*replica) {
	t.Helper()
	heads := func(r *replica) map[string][]string {
		byDAG := map[string][]string{}
		for _, h := range r.store.crdt.InternalStats(context.Background()).Heads {
			byDAG[h.DAGName] = append(byDAG[h.DAGName], h.Cid.String())
		}
		return byDAG
	}
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		want := heads(rs[0])
		for name, cids := range want {
			assert.Len(collect, cids, 1, name)
		}
		for _, r := range rs[1:] {
			assert.Equal(collect, want, heads(r), r.name)
		}
	}, replicationTimeout, replicationTick, "the replicas never settled on one head per DAG")
}

// newCluster connects the hosts before their stores start, as the bootstrap
// peers a member node already holds when its store comes up.
func newCluster(t *testing.T, names ...string) []*replica {
	t.Helper()
	if testing.Short() {
		t.Skip("multi-host replication")
	}
	rs := make([]*replica, len(names))
	for i, name := range names {
		rs[i] = newReplica(t, name)
	}
	connectAll(t, rs...)
	for _, r := range rs {
		r.open(t)
	}
	awaitMesh(t, rs...)
	return rs
}

func currentBucket() int64 { return time.Now().Unix() / 3600 }

func withCount(rec domain.RatingRecord, count uint32) domain.RatingRecord {
	rec.Offences = []domain.OffenceCount{{Kind: "bad_signature", Count: count}}
	rec.UpdatedAt = time.Unix(rec.Bucket*3600+int64(count)*60, 0).UTC()
	return rec
}

func keyFields(rec domain.RatingRecord) domain.RatingRecord {
	return domain.RatingRecord{
		PeerID: rec.PeerID, ObserverID: rec.ObserverID, Dimension: rec.Dimension,
		Bucket: rec.Bucket, Generation: rec.Generation,
	}
}

func TestReplication_ARecordWrittenOnOneReplicaReachesEveryOther(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a, b, c := rs[0], rs[1], rs[2]

	rec := ratingRecord(a.id(), observedPeer, "net", currentBucket())
	require.NoError(t, a.store.Put(rec))

	for _, r := range []*replica{b, c} {
		r.eventuallyHooked(t, r.puts, rec)
		assert.Equal(t, []domain.RatingRecord{rec}, r.puts.snapshot(), "%s: one merge carrying the identical record", r.name)
		r.eventuallyLists(t, observedPeer, rec)
	}
}

func TestReplication_RecordsBySeveralObserversAboutOnePeerConvergeOnEveryReplica(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")

	// written back to back, so the replicas start from concurrent heads
	var want []domain.RatingRecord
	for _, r := range rs {
		rec := ratingRecord(r.id(), observedPeer, "net", currentBucket())
		require.NoError(t, r.store.Put(rec))
		want = append(want, rec)
	}

	for _, r := range rs {
		r.eventuallyLists(t, observedPeer, want...)
		for _, rec := range want {
			r.eventuallyHooked(t, r.puts, rec)
		}
	}
}

func TestReplication_AnOverwriteConvergesToTheNewestValueEverywhere(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a := rs[0]

	first := ratingRecord(a.id(), observedPeer, "net", currentBucket())
	require.NoError(t, a.store.Put(first))
	for _, r := range rs {
		r.eventuallyLists(t, observedPeer, first)
	}

	newest := withCount(first, 3)
	require.NoError(t, a.store.Put(withCount(first, 2)))
	require.NoError(t, a.store.Put(newest))

	for _, r := range rs {
		r.eventuallyLists(t, observedPeer, newest)
		puts := r.puts.snapshot()
		assert.Equal(t, newest, puts[len(puts)-1], "%s: an older value merged late must not win", r.name)
	}
}

func TestReplication_DeleteExpiredDropsTheDayOnlyWhereItRuns(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a, b, c := rs[0], rs[1], rs[2]
	now := currentBucket()

	aOld := ratingRecord(a.id(), observedPeer, "net", now-48)
	aNew := ratingRecord(a.id(), observedPeer, "net", now)
	aOldApp := ratingRecord(a.id(), observedPeer, "app", now-48)
	bOld := ratingRecord(b.id(), observedPeer, "net", now-48)
	cOld := ratingRecord(c.id(), observedPeer, "net", now-48)
	require.NoError(t, a.store.Put(aOld, aNew, aOldApp))
	require.NoError(t, b.store.Put(bOld))
	require.NoError(t, c.store.Put(cOld))
	all := []domain.RatingRecord{aOld, aNew, aOldApp, bOld, cOld}
	for _, r := range rs {
		r.eventuallyLists(t, observedPeer, all...)
	}

	require.NoError(t, a.store.DeleteExpired("net", now-24))
	a.eventuallyLists(t, observedPeer, aNew, aOldApp)
	for _, rec := range []domain.RatingRecord{aOld, bOld, cOld} {
		a.eventuallyHooked(t, a.deletes, keyFields(rec))
	}
	for _, r := range []*replica{b, c} {
		r.eventuallyLists(t, observedPeer, all...)
		assert.Empty(t, r.deletes.snapshot(), "%s: a day dropped elsewhere is not dropped here", r.name)
	}

	for _, r := range []*replica{b, c} {
		require.NoError(t, r.store.DeleteExpired("net", now-24))
		r.eventuallyLists(t, observedPeer, aNew, aOldApp)
	}
}

func TestReplication_ARecordUnderAForeignKeyNeverSurfacesOnOtherReplicas(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a, b, c := rs[0], rs[1], rs[2]
	now := currentBucket()

	// A files its record about otherPeer under a key about observedPeer
	key, err := recordKey(ratingRecord(a.id(), observedPeer, "net", now))
	require.NoError(t, err)
	forged, err := json.Marshal(ratingRecord(a.id(), otherPeer, "net", now))
	require.NoError(t, err)
	publishInEpoch(t, a.store, key.String(), forged, dayDAG("net", now))

	marker := ratingRecord(a.id(), observedPeer, "app", now)
	require.NoError(t, a.store.Put(marker))

	for _, r := range []*replica{b, c} {
		r.eventuallyHooked(t, r.puts, marker)
		has, err := r.store.crdt.Has(context.Background(), key)
		require.NoError(t, err)
		assert.False(t, has, "%s: a value whose content does not match its key is refused before the merge", r.name)

		got, err := r.store.List(observedPeer)
		require.NoError(t, err)
		assert.Equal(t, []domain.RatingRecord{marker}, got, "%s: the forged value is not evidence about observedPeer", r.name)
		got, err = r.store.List(otherPeer)
		require.NoError(t, err)
		assert.Empty(t, got, "%s: nor about the peer it names", r.name)
		assert.Equal(t, []domain.RatingRecord{marker}, r.puts.snapshot(), "%s: the forged value never reached a hook", r.name)
	}
}

func TestReplication_ALateJoinerConvergesOnTheFullHistory(t *testing.T) {
	for _, tc := range []struct {
		name         string
		connectFirst bool
	}{
		{name: "store started before it connects"},
		{name: "store started after it connects", connectFirst: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := newCluster(t, "A", "B", "C")
			a, b, c := rs[0], rs[1], rs[2]
			now := currentBucket()

			write := func(r *replica, rec domain.RatingRecord) {
				require.NoError(t, r.store.Put(rec))
				settle(t, rs...)
			}
			aOld := ratingRecord(a.id(), observedPeer, "net", now-48)
			aNet := ratingRecord(a.id(), observedPeer, "net", now)
			bApp := ratingRecord(b.id(), observedPeer, "app", now)
			cMod := ratingRecord(c.id(), observedPeer, "mod", now)
			write(a, aOld)
			write(a, aNet)
			write(b, bApp)
			write(c, cMod)
			aNet = withCount(aNet, 4)
			write(a, aNet)
			for _, r := range rs {
				require.NoError(t, r.store.DeleteExpired("net", now-24))
			}
			history := []domain.RatingRecord{aNet, bApp, cMod}
			for _, r := range rs {
				r.eventuallyLists(t, observedPeer, history...)
			}

			d := newReplica(t, "D")
			if tc.connectFirst {
				connectAll(t, d, a, b, c)
				d.open(t)
			} else {
				d.open(t)
				connectAll(t, d, a, b, c)
			}
			awaitMesh(t, a, b, c, d)

			next := []domain.RatingRecord{
				ratingRecord(b.id(), otherPeer, "net", now),
				ratingRecord(b.id(), otherPeer, "app", now),
				ratingRecord(c.id(), otherPeer, "mod", now),
			}
			require.NoError(t, b.store.Put(next[0], next[1]))
			require.NoError(t, c.store.Put(next[2]))

			d.eventuallyLists(t, otherPeer, next...)
			d.eventuallyLists(t, observedPeer, history...)
		})
	}
}

func restartProcess(t *testing.T, r *replica, peers ...*replica) {
	t.Helper()
	priv := r.host.Peerstore().PrivKey(r.host.ID())
	require.NotNil(t, priv)
	id := r.host.ID()
	r.stop()
	for _, p := range peers {
		require.Eventually(t, func() bool { return !p.mesh.has(GossipTopic, id) },
			replicationTimeout, replicationTick, "%s kept the stopped %s in its mesh", p.name, r.name)
	}
	r.boot(t, libp2p.Identity(priv))
	r.open(t)
}

// restartStore reopens only the store, on the host and pubsub still running.
func restartStore(t *testing.T, r *replica, _ ...*replica) {
	t.Helper()
	r.closeStore(t)
	r.open(t)
}

func TestReplication_ARestartedReplicaKeepsItsRecordsAndReplicatesBothWays(t *testing.T) {
	for _, tc := range []struct {
		name    string
		skip    string
		restart func(t *testing.T, r *replica, peers ...*replica)
	}{
		{name: "new process with the same identity and datastore", restart: restartProcess},
		{
			name: "store reopened on the running host",
			skip: "known bug: Store.Close never closes the bitswap exchange store.New starts, so the streams " +
				"peers already hold keep reaching the dead one and they never fetch blocks written after the reopen",
			restart: restartStore,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			rs := newCluster(t, "A", "B", "C")
			a, b, c := rs[0], rs[1], rs[2]
			now := currentBucket()

			aRec := ratingRecord(a.id(), observedPeer, "net", now)
			bRec := ratingRecord(b.id(), observedPeer, "app", now)
			require.NoError(t, a.store.Put(aRec))
			require.NoError(t, b.store.Put(bRec))
			for _, r := range rs {
				r.eventuallyLists(t, observedPeer, aRec, bRec)
			}

			tc.restart(t, a, b, c)

			got, err := a.store.List(observedPeer)
			require.NoError(t, err)
			assert.ElementsMatch(t, []domain.RatingRecord{aRec, bRec}, got, "the datastore alone holds what A had")

			connect(t, a, b)
			connect(t, a, c)
			awaitMesh(t, a, b, c)

			aNext := ratingRecord(a.id(), otherPeer, "net", now)
			bNext := ratingRecord(b.id(), otherPeer, "app", now)
			require.NoError(t, a.store.Put(aNext))
			require.NoError(t, b.store.Put(bNext))

			for _, r := range rs {
				r.eventuallyLists(t, otherPeer, aNext, bNext)
			}
			a.eventuallyHooked(t, a.puts, bNext)
		})
	}
}

func TestReplication_AForgedRecordCannotDisplaceTheObserversOwn(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a, b, c := rs[0], rs[1], rs[2]
	now := currentBucket()

	honest := ratingRecord(a.id(), observedPeer, "net", now)
	require.NoError(t, a.store.Put(honest))
	b.eventuallyLists(t, observedPeer, honest)

	forged := honest
	forged.Offences = []domain.OffenceCount{{Kind: "bad_signature", Count: 1000}}
	forged.Signature = forgedSignature
	key, err := recordKey(forged)
	require.NoError(t, err)
	value, err := json.Marshal(forged)
	require.NoError(t, err)
	publishInEpoch(t, b.store, key.String(), value, dayDAG("net", now))

	marker := ratingRecord(b.id(), otherPeer, "net", now)
	require.NoError(t, b.store.Put(marker))
	c.eventuallyLists(t, otherPeer, marker)

	got, err := c.store.List(observedPeer)
	require.NoError(t, err)
	assert.Equal(t, []domain.RatingRecord{honest}, got,
		"a record its observer did not sign is refused before it can win the key")
}

func TestReplication_ARecordOutsideItsDayDAGIsRefused(t *testing.T) {
	rs := newCluster(t, "A", "B")
	a, b := rs[0], rs[1]
	now := currentBucket()

	old := ratingRecord(a.id(), observedPeer, "net", now-60*bucketsPerDay)
	key, err := recordKey(old)
	require.NoError(t, err)
	value, err := json.Marshal(old)
	require.NoError(t, err)
	publishInEpoch(t, a.store, key.String(), value, dayDAG("net", now))

	marker := ratingRecord(a.id(), otherPeer, "net", now)
	require.NoError(t, a.store.Put(marker))
	b.eventuallyLists(t, otherPeer, marker)

	has, err := b.store.crdt.Has(context.Background(), key)
	require.NoError(t, err)
	assert.False(t, has, "a record filed under another day's DAG would outlive its own day")
}

func TestReplication_AReplayedOlderRecordCannotWinTheKey(t *testing.T) {
	rs := newCluster(t, "A", "B", "C")
	a, b, c := rs[0], rs[1], rs[2]
	now := currentBucket()

	first := ratingRecord(a.id(), observedPeer, "net", now)
	require.NoError(t, a.store.Put(first))
	latest := first
	latest.Offences = []domain.OffenceCount{{Kind: "bad_signature", Count: 5}}
	latest.UpdatedAt = first.UpdatedAt.Add(time.Minute)
	require.NoError(t, a.store.Put(latest))
	b.eventuallyLists(t, observedPeer, latest)
	c.eventuallyLists(t, observedPeer, latest)

	key, err := recordKey(first)
	require.NoError(t, err)
	replay, err := json.Marshal(first)
	require.NoError(t, err)
	publishInEpoch(t, b.store, key.String(), replay, dayDAG("net", now))
	marker := ratingRecord(b.id(), otherPeer, "net", now)
	require.NoError(t, b.store.Put(marker))
	c.eventuallyLists(t, otherPeer, marker)

	got, err := c.store.List(observedPeer)
	require.NoError(t, err)
	assert.Equal(t, []domain.RatingRecord{latest}, got, "an observer's earlier record, replayed at a higher priority, does not undo its latest")
}
