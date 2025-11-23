package database

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/Warp-net/warpnet/database/local"
	"github.com/Warp-net/warpnet/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/suite"
	"go.uber.org/goleak"
)

const testUserID = "01BX5ZZKBKACTAV9WEVGEMTEST"

type ChatRepoSuite struct {
	suite.Suite

	repo *ChatRepo
	db   *local.DB
}

func (s *ChatRepoSuite) SetupSuite() {
	db, err := local.New("", local.DefaultOptions().WithInMemory(true))
	s.Require().NoError(err)

	s.db = db

	authRepo := NewAuthRepo(db)
	err = authRepo.Authenticate(rand.Text(), rand.Text())
	s.Require().NoError(err)

	s.repo = NewChatRepo(db)
}

func (s *ChatRepoSuite) TearDownSuite() {
	s.db.Close()
}

func (s *ChatRepoSuite) TestCreateAndGetChat() {
	ownerID := testUserID
	otherID := ulid.Make().String()

	chat, err := s.repo.CreateChat(nil, ownerID, otherID)
	s.NoError(err)
	defer s.repo.DeleteChat(chat.Id)

	fetched, err := s.repo.GetChat(chat.Id)
	s.NoError(err)
	s.Equal(chat.Id, fetched.Id)
	s.Equal(chat.OwnerId, fetched.OwnerId)
	s.Equal(chat.OtherUserId, fetched.OtherUserId)


}

func (s *ChatRepoSuite) TestDeleteChat() {
	ownerID := testUserID
	otherID := ulid.Make().String()

	chat, err := s.repo.CreateChat(nil, ownerID, otherID)
	s.NoError(err)
	s.NotEmpty(chat.Id)

	err = s.repo.DeleteChat(chat.Id)
	s.NoError(err)

	deleted, err := s.repo.GetChat(chat.Id)
	s.Error(err)
	s.Empty(deleted.Id)
}

func (s *ChatRepoSuite) TestGetUserChats() {
	userID := testUserID

	for range 3 {
		other := ulid.Make().String()
		_, err := s.repo.CreateChat(nil, userID, other)
		s.NoError(err)
	}

	limit := uint64(10)
	chats, cursor, err := s.repo.GetUserChats(userID, &limit, nil)
	s.NoError(err)
	s.Len(chats, 3)
	s.Equal("end", cursor)
}

func (s *ChatRepoSuite) TestCreateAndGetMessage() {
	ownerID := testUserID
	otherID := ulid.Make().String()

	chat, err := s.repo.CreateChat(nil, ownerID, otherID)
	s.NoError(err)
	defer s.repo.DeleteChat(chat.Id)

	msg := domain.ChatMessage{
		ChatId: chat.Id,
		Text:   "hello",
	}

	created, err := s.repo.CreateMessage(msg)
	s.NoError(err)

	got, err := s.repo.GetMessage(chat.Id, created.Id)
	s.NoError(err)
	s.Equal(msg.Text, got.Text)
}

func (s *ChatRepoSuite) TestListMessages() {
	ownerID := testUserID
	otherID := ulid.Make().String()

	chat, err := s.repo.CreateChat(nil, ownerID, otherID)
	s.NoError(err)
	s.NotEmpty(chat.Id)
	defer s.repo.DeleteChat(chat.Id)

	for i := 0; i < 5; i++ { //nolint:modernize
		msg := domain.ChatMessage{
			ChatId:    chat.Id,
			Text:      "msg",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Second),
		}
		_, err := s.repo.CreateMessage(msg)
		s.NoError(err)
	}

	limit := uint64(10)
	msgs, cursor, err := s.repo.ListMessages(chat.Id, &limit, nil)
	s.NoError(err)
	s.Len(msgs, 5)
	s.Equal("end", cursor)
}

func (s *ChatRepoSuite) TestDeleteMessage() {
	ownerID := testUserID
	otherID := ulid.Make().String()

	chat, err := s.repo.CreateChat(nil, ownerID, otherID)
	s.NoError(err)
	s.NotEmpty(chat.Id)
	defer s.repo.DeleteChat(chat.Id)

	msg := domain.ChatMessage{
		ChatId: chat.Id,
		Text:   "to delete",
	}
	created, err := s.repo.CreateMessage(msg)
	s.NoError(err)

	err = s.repo.DeleteMessage(chat.Id, created.Id)
	s.NoError(err)

	got, err := s.repo.GetMessage(chat.Id, created.Id)
	s.Error(err)
	s.Empty(got.Text)
}

func TestChatRepoSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	suite.Run(t, new(ChatRepoSuite))
}
