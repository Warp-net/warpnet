//nolint:all
package database

import (
	"context"
	"testing"

	"go.uber.org/goleak"

	ds "github.com/Warp-net/warpnet/database/datastore"
	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RatingRepoTestSuite struct {
	suite.Suite

	db *local_store.DB
}

func (s *RatingRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)
}

func (s *RatingRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *RatingRepoTestSuite) TestNewRatingRepo() {
	repo := NewRatingRepo(s.db)
	assert.NotNil(s.T(), repo)
}

func (s *RatingRepoTestSuite) TestRatingRepoIsSeparateFromStats() {
	ctx := context.Background()
	key := ds.NewKey("/RATING/record/peer/observer/net/1/gen")

	rating := NewRatingRepo(s.db)
	stats := NewStatsRepo(s.db)
	s.Require().NoError(rating.Put(ctx, key, []byte("record")))

	got, err := rating.Get(ctx, key)
	s.Require().NoError(err)
	assert.Equal(s.T(), []byte("record"), got)

	_, err = stats.Get(ctx, key)
	assert.ErrorIs(s.T(), err, ds.ErrNotFound, "the two CRDTs must not see each other's keys")
}

func (s *RatingRepoTestSuite) TestNewReposDropTheOldSingleDAGHistory() {
	ctx := context.Background()
	for _, key := range []string{"/RATING/h/head", "/RATING/s/k/record/v", "/CRDT/s/k/counter/v"} {
		s.Require().NoError(s.db.Set(local_store.DatabaseKey(key), []byte("old")))
	}
	rating := NewRatingRepo(s.db)
	stats := NewStatsRepo(s.db)
	s.Require().NoError(rating.Put(ctx, ds.NewKey("/h/day"), []byte("new")))
	s.Require().NoError(stats.Put(ctx, ds.NewKey("/h/day"), []byte("new")))

	for _, key := range []string{"/RATING/h/head", "/RATING/s/k/record/v", "/CRDT/s/k/counter/v"} {
		_, err := s.db.Get(local_store.DatabaseKey(key))
		assert.True(s.T(), local_store.IsNotFoundError(err), "%s is old history and goes", key)
	}

	NewRatingRepo(s.db)
	NewStatsRepo(s.db)
	for _, repo := range []ds.Datastore{rating, stats} {
		got, err := repo.Get(ctx, ds.NewKey("/h/day"))
		s.Require().NoError(err, "a later start must not drop the day DAGs")
		assert.Equal(s.T(), []byte("new"), got)
	}
}

func TestRatingRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(RatingRepoTestSuite))
}
