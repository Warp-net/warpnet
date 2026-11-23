package node

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type closedStoreProbe struct {
	Storer

	isClosed         atomic.Bool
	servedAfterClose atomic.Int64
}

func (p *closedStoreProbe) countServed(err error) {
	if err == nil && p.isClosed.Load() {
		p.servedAfterClose.Add(1)
	}
}

func (p *closedStoreProbe) NewTxn() (local_store.WarpTransactioner, error) {
	txn, err := p.Storer.NewTxn()
	p.countServed(err)
	return txn, err
}

func (p *closedStoreProbe) NewReadTxn() (local_store.WarpTransactioner, error) {
	txn, err := p.Storer.NewReadTxn()
	p.countServed(err)
	return txn, err
}

func (p *closedStoreProbe) Get(key local_store.DatabaseKey) ([]byte, error) {
	value, err := p.Storer.Get(key)
	p.countServed(err)
	return value, err
}

func (p *closedStoreProbe) Set(key local_store.DatabaseKey, value []byte) error {
	err := p.Storer.Set(key, value)
	p.countServed(err)
	return err
}

func (p *closedStoreProbe) SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error {
	err := p.Storer.SetWithTTL(key, value, ttl)
	p.countServed(err)
	return err
}

func (p *closedStoreProbe) Delete(key local_store.DatabaseKey) error {
	err := p.Storer.Delete(key)
	p.countServed(err)
	return err
}

func callAsOwner(m *MemberNode, route string, body any) error {
	bt, err := json.Marshal(body)
	if err != nil {
		return err
	}
	id := m.node.BaseNodeInfo().ID
	msg := event.Message{
		Body: bt, MessageId: uuid.New().String(), NodeId: id.String(),
		Destination: route, Timestamp: time.Now().UTC(), Version: "0.0.0",
	}
	msg.Signature = security.Sign(m.privKey, msg.SigningBytes())
	_, err = m.SelfStream(id, id, stream.WarpRoute(route), msg)
	return err
}

func TestClosedStoreRefusesLateCalls(t *testing.T) {
	for round := range 5 {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			db, err := local_store.New(t.TempDir(), local_store.DefaultOptions())
			require.NoError(t, err)
			require.NoError(t, db.Run("test", "Test1234$"))
			authRepo := database.NewAuthRepo(db, "testnet")
			require.NoError(t, authRepo.Authenticate("test", "Test1234$"))
			_, err = authRepo.SetOwner(domain.Owner{UserId: "owner-1", Login: "owner"})
			require.NoError(t, err)

			probe := &closedStoreProbe{Storer: db}
			privKey, ownNodeId := testKeyAndID(t)
			psk, err := security.GeneratePSK("testnet", semver.MustParse("0.0.0"))
			require.NoError(t, err)
			owner, err := NewMemberNode(context.Background(), privKey, psk, ownNodeId, authRepo, probe, nil)
			require.NoError(t, err)
			owner.mdnsService = noopMDNS{}
			serveStreams(t, owner)

			peer, _ := newOwnedMemberNode(t, "peer-1")
			serveStreams(t, peer)
			require.NoError(t, peer.Connect(warpnet.WarpAddrInfo{
				ID: owner.node.Node().ID(), Addrs: owner.node.Node().Addrs(),
			}))
			ownerID := owner.node.Node().ID().String()

			var (
				isDetached, isDone atomic.Bool
				ownerCalls         sync.WaitGroup
				peerCalls          sync.WaitGroup
				served             atomic.Int64
			)
			for w := range 8 {
				ownerCalls.Go(func() {
					for i := 0; !isDetached.Load(); i++ {
						var err error
						switch i % 3 {
						case 0:
							err = callAsOwner(owner, event.PRIVATE_POST_TWEET, event.NewTweetEvent{
								UserId: "owner-1", Username: "owner", Text: fmt.Sprintf("load %d %d", w, i),
							})
						case 1:
							err = callAsOwner(owner, event.PRIVATE_GET_TIMELINE, event.GetTimelineEvent{UserId: "owner-1"})
						default:
							err = callAsOwner(owner, event.PUBLIC_GET_TWEETS, event.GetAllTweetsEvent{UserId: "owner-1"})
						}
						if err == nil {
							served.Add(1)
						}
					}
				})
				peerCalls.Go(func() {
					for !isDone.Load() {
						_, _ = peer.GenericStream(ownerID, event.PUBLIC_GET_TWEETS, event.GetAllTweetsEvent{UserId: "owner-1"})
					}
				})
			}

			require.Eventually(t, func() bool { return served.Load() >= 100 }, 30*time.Second, 10*time.Millisecond)

			isDetached.Store(true)
			stopped := make(chan struct{})
			go func() {
				owner.Stop()
				db.Close()
				probe.isClosed.Store(true)
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-time.After(30 * time.Second):
				t.Fatal("logout hangs")
			}

			finished := make(chan struct{})
			go func() { ownerCalls.Wait(); close(finished) }()
			select {
			case <-finished:
			case <-time.After(30 * time.Second):
				t.Fatal("an owner call hangs in the closed store")
			}
			isDone.Store(true)
			peerCalls.Wait()
			time.Sleep(200 * time.Millisecond)

			require.Zero(t, probe.servedAfterClose.Load(), "the closed store served a call")

			require.NoError(t, db.Run("test", "Test1234$"))
			require.NoError(t, authRepo.Authenticate("test", "Test1234$"))
			again := newMemberNodeOn(t, db, authRepo)
			serveStreams(t, again)
			require.NoError(t, callAsOwner(again, event.PRIVATE_GET_TIMELINE, event.GetTimelineEvent{UserId: "owner-1"}))
			again.Stop()
			db.Close()
		})
	}
}
