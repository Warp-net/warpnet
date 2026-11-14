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

func (s *RatingRepoTestSuite) TestOldCRDTHistoryIsLeftUnread() {
	ctx := context.Background()
	s.Require().NoError(s.db.Set("/RATING/h/head", []byte("old rating")))
	s.Require().NoError(s.db.Set("/CRDT/h/head", []byte("old stats")))

	for _, repo := range []ds.Datastore{NewRatingRepo(s.db), NewStatsRepo(s.db)} {
		_, err := repo.Get(ctx, ds.NewKey("/h/head"))
		assert.ErrorIs(s.T(), err, ds.ErrNotFound, "the day-DAG CRDTs start from an empty namespace")
	}
	got, err := s.db.Get("/RATING/h/head")
	s.Require().NoError(err)
	assert.Equal(s.T(), []byte("old rating"), got, "the old history is not touched")

	ctx2 := context.Background()
	s.Require().NoError(NewRatingRepo(s.db).Put(ctx2, ds.NewKey("/h/day"), []byte("new")))
	_, err = s.db.Get("/RATING/DAYS/h/day")
	s.Require().NoError(err, "day DAG keys are a slash path, as badger-gui reads them")
}

func TestRatingRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(RatingRepoTestSuite))
}
