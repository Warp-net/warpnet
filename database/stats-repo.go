package database

import (
	ds "github.com/Warp-net/warpnet/database/datastore"
	local_store "github.com/Warp-net/warpnet/database/local-store"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

type StatsStorer interface {
	NewTxn() (local_store.WarpTransactioner, error)
	Get(key local_store.DatabaseKey) ([]byte, error)
	GetExpiration(key local_store.DatabaseKey) (uint64, error)
	GetSize(key local_store.DatabaseKey) (int64, error)
	Sync() error
	IsClosed() bool
	InnerDB() *local_store.WarpDB
	SetWithTTL(key local_store.DatabaseKey, value []byte, ttl time.Duration) error
	Set(key local_store.DatabaseKey, value []byte) error
	Delete(key local_store.DatabaseKey) error
	DropPrefix(prefix local_store.DatabaseKey) error
}

const (
	statsPrefix = "STATS_DAYS"
	// oldStatsPrefix held the stats CRDT before it kept a DAG per day and
	// signed its counters.
	oldStatsPrefix = "/CRDT/"
)

// NewStatsRepo backs the stats CRDT. It drops the old unsigned counters,
// which no one reads any more.
func NewStatsRepo(db StatsStorer) ds.Datastore {
	if err := db.DropPrefix(oldStatsPrefix); err != nil {
		log.Errorf("stats repo: drop old counters: %v", err)
	}
	prefix := statsPrefix
	if !strings.HasPrefix(prefix, requiredPrefixSlash) {
		prefix = requiredPrefixSlash + prefix
	}
	nr := &NodeRepo{
		db:       db,
		prefix:   prefix,
		stopChan: make(chan struct{}),
	}

	return nr
}
