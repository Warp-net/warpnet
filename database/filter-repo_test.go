//nolint:all
package database

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/Warp-net/warpnet/database/local-store"
	"github.com/Warp-net/warpnet/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type FilterRepoTestSuite struct {
	suite.Suite

	db   *local_store.DB
	repo *FilterRepo
}

func (s *FilterRepoTestSuite) SetupSuite() {
	var err error
	s.db, err = local_store.New("", local_store.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)
	authRepo := NewAuthRepo(s.db, "test")
	s.Require().NoError(authRepo.Authenticate("test", "test"))
	s.repo = NewFilterRepo(s.db)
}

func (s *FilterRepoTestSuite) TearDownSuite() {
	s.db.Close()
}

func (s *FilterRepoTestSuite) TestCreateGetList() {
	user := uuid.New().String()
	created, err := s.repo.Create(user, domain.Filter{
		Title:   "spoilers",
		Context: []domain.FilterContext{domain.FilterContextHome},
		Action:  domain.FilterActionHide,
		Keywords: []domain.FilterKeyword{
			{Keyword: "season finale", WholeWord: false},
		},
	})
	s.Require().NoError(err)
	s.NotEmpty(created.Id, "id should be auto-generated")
	s.Equal(user, created.UserId)
	s.NotEmpty(created.Keywords[0].Id, "keyword id should be auto-generated on create")

	fetched, err := s.repo.Get(user, created.Id)
	s.Require().NoError(err)
	s.Equal(created.Id, fetched.Id)
	s.Equal("spoilers", fetched.Title)
	s.Len(fetched.Keywords, 1)

	limit := uint64(10)
	list, _, err := s.repo.List(user, &limit, nil)
	s.Require().NoError(err)
	s.Len(list, 1)
}

func (s *FilterRepoTestSuite) TestUpdatePreservesKeywords() {
	user := uuid.New().String()
	created, err := s.repo.Create(user, domain.Filter{
		Title:    "x",
		Keywords: []domain.FilterKeyword{{Keyword: "a"}, {Keyword: "b"}},
	})
	s.Require().NoError(err)

	updated, err := s.repo.Update(user, domain.Filter{Id: created.Id, Title: "new title"})
	s.Require().NoError(err)
	s.Equal("new title", updated.Title)
	s.Len(updated.Keywords, 2, "Update should not overwrite the keywords array")
}

func (s *FilterRepoTestSuite) TestDelete() {
	user := uuid.New().String()
	created, err := s.repo.Create(user, domain.Filter{Title: "x"})
	s.Require().NoError(err)
	s.Require().NoError(s.repo.Delete(user, created.Id))
	_, err = s.repo.Get(user, created.Id)
	s.ErrorIs(err, ErrFilterNotFound)
}

func (s *FilterRepoTestSuite) TestKeywordSubCRUD() {
	user := uuid.New().String()
	f, err := s.repo.Create(user, domain.Filter{Title: "k"})
	s.Require().NoError(err)

	kw, err := s.repo.AddKeyword(user, f.Id, domain.FilterKeyword{Keyword: "spoil", WholeWord: false})
	s.Require().NoError(err)
	s.NotEmpty(kw.Id)

	updated, err := s.repo.UpdateKeyword(user, domain.FilterKeyword{Id: kw.Id, Keyword: "spoiler", WholeWord: true})
	s.Require().NoError(err)
	s.Equal("spoiler", updated.Keyword)
	s.True(updated.WholeWord)

	s.Require().NoError(s.repo.DeleteKeyword(user, kw.Id))

	got, err := s.repo.Get(user, f.Id)
	s.Require().NoError(err)
	s.Empty(got.Keywords)
}

func (s *FilterRepoTestSuite) TestDeleteKeywordNonexistentIsNoop() {
	user := uuid.New().String()
	s.Require().NoError(s.repo.DeleteKeyword(user, uuid.New().String()))
}

func (s *FilterRepoTestSuite) TestValidation() {
	_, err := s.repo.Create("", domain.Filter{Title: "x"})
	s.Error(err)
	_, err = s.repo.Create("u", domain.Filter{})
	s.Error(err)

	_, err = s.repo.Update("", domain.Filter{Id: "x"})
	s.Error(err)
	_, err = s.repo.Update("u", domain.Filter{})
	s.Error(err)

	s.Error(s.repo.Delete("", "x"))
	s.Error(s.repo.Delete("u", ""))

	_, _, err = s.repo.List("", nil, nil)
	s.Error(err)

	_, err = s.repo.AddKeyword("u", uuid.New().String(), domain.FilterKeyword{})
	s.Error(err) // empty keyword

	s.Error(s.repo.DeleteKeyword("u", ""))
}

func (s *FilterRepoTestSuite) TestIsolatedByUser() {
	u1 := uuid.New().String()
	u2 := uuid.New().String()
	_, err := s.repo.Create(u1, domain.Filter{Title: "for u1"})
	s.Require().NoError(err)

	limit := uint64(10)
	u1list, _, err := s.repo.List(u1, &limit, nil)
	s.Require().NoError(err)
	s.Len(u1list, 1)

	u2list, _, err := s.repo.List(u2, &limit, nil)
	s.Require().NoError(err)
	s.Empty(u2list)
}

func TestFilterRepoTestSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	suite.Run(t, new(FilterRepoTestSuite))
}
