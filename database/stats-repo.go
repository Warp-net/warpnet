package database

import (
	ds "github.com/Warp-net/warpnet/database/datastore"
	local_store "github.com/Warp-net/warpnet/database/local-store"
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
}

const (
	crdtPrefix   = "CRDT"
	ratingPrefix = "RATING"
)

func NewStatsRepo(db StatsStorer) ds.Datastore {
	return newCRDTBackedRepo(db, crdtPrefix)
}

// NewRatingRepo backs the rating CRDT. Its own prefix keeps a rating
// wipe from touching stat counters and vice versa.
func NewRatingRepo(db StatsStorer) ds.Datastore {
	return newCRDTBackedRepo(db, ratingPrefix)
}

func newCRDTBackedRepo(db StatsStorer, prefix string) ds.Datastore {
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
