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

func (s *RatingRepoTestSuite) TestClearDropsOnlyThisRepo() {
	ctx := context.Background()
	rating := NewRatingRepo(s.db)
	stats := NewStatsRepo(s.db)
	for _, key := range []string{"/s/k/record/a/v", "/blocks/abc", "/h/head"} {
		s.Require().NoError(rating.Put(ctx, ds.NewKey(key), []byte("x")))
	}
	kept := ds.NewKey("/s/k/counter/v")
	s.Require().NoError(stats.Put(ctx, kept, []byte("y")))

	s.Require().NoError(rating.(*NodeRepo).Clear())

	results, err := rating.Query(ctx, ds.Query{KeysOnly: true})
	s.Require().NoError(err)
	left, err := results.Rest()
	s.Require().NoError(err)
	assert.Empty(s.T(), left, "every key of the repo goes")
	got, err := stats.Get(ctx, kept)
	s.Require().NoError(err)
	assert.Equal(s.T(), []byte("y"), got, "another repo in the same database is untouched")

	s.Require().NoError(rating.Put(ctx, ds.NewKey("/after"), []byte("z")), "the repo is usable after a clear")
}

func TestRatingRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(RatingRepoTestSuite))
}
