//nolint:all
package database

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type StatsRepoTestSuite struct {
	suite.Suite

	db *local_store.DB
}

func (s *StatsRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	err = s.db.Run("test", "test")
	s.Require().NoError(err)
}

func (s *StatsRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *StatsRepoTestSuite) TestNewStatsRepo() {
	repo := NewStatsRepo(s.db)
	assert.NotNil(s.T(), repo)
}

func TestStatsRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(StatsRepoTestSuite))
}
