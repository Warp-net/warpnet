//nolint:all
package handler

import (
	"errors"
	"testing"

	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
)

type stubFilterRepo struct {
	createFn   func(userId string, f domain.Filter) (domain.Filter, error)
	getFn      func(userId, filterId string) (domain.Filter, error)
	updateFn   func(userId string, f domain.Filter) (domain.Filter, error)
	deleteFn   func(userId, filterId string) error
	listFn     func(userId string, limit *uint64, cursor *string) ([]domain.Filter, string, error)
	addKwFn    func(userId, filterId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
	updateKwFn func(userId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
	deleteKwFn func(userId, keywordId string) error
}

func (s stubFilterRepo) Create(u string, f domain.Filter) (domain.Filter, error) {
	if s.createFn != nil {
		return s.createFn(u, f)
	}
	if f.Id == "" {
		f.Id = "filt-id"
	}
	f.UserId = u
	return f, nil
}

func (s stubFilterRepo) Get(u, fid string) (domain.Filter, error) {
	if s.getFn != nil {
		return s.getFn(u, fid)
	}
	return domain.Filter{Id: fid, UserId: u, Title: "x"}, nil
}

func (s stubFilterRepo) Update(u string, f domain.Filter) (domain.Filter, error) {
	if s.updateFn != nil {
		return s.updateFn(u, f)
	}
	return f, nil
}

func (s stubFilterRepo) Delete(u, fid string) error {
	if s.deleteFn != nil {
		return s.deleteFn(u, fid)
	}
	return nil
}

func (s stubFilterRepo) List(u string, l *uint64, c *string) ([]domain.Filter, string, error) {
	if s.listFn != nil {
		return s.listFn(u, l, c)
	}
	return nil, "end", nil
}

func (s stubFilterRepo) AddKeyword(u, fid string, kw domain.FilterKeyword) (domain.FilterKeyword, error) {
	if s.addKwFn != nil {
		return s.addKwFn(u, fid, kw)
	}
	if kw.Id == "" {
		kw.Id = "kw-id"
	}
	return kw, nil
}

func (s stubFilterRepo) UpdateKeyword(u string, kw domain.FilterKeyword) (domain.FilterKeyword, error) {
	if s.updateKwFn != nil {
		return s.updateKwFn(u, kw)
	}
	return kw, nil
}

func (s stubFilterRepo) DeleteKeyword(u, kid string) error {
	if s.deleteKwFn != nil {
		return s.deleteKwFn(u, kid)
	}
	return nil
}

func TestStreamGetFilterHandler(t *testing.T) {
	t.Run("invalid payload", func(t *testing.T) {
		_, err := StreamGetFilterHandler(stubFilterRepo{})([]byte("{"), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamGetFilterHandler(stubFilterRepo{})(marshal(t, event.GetFilterEvent{FilterId: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty filter id", func(t *testing.T) {
		_, err := StreamGetFilterHandler(stubFilterRepo{})(marshal(t, event.GetFilterEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetFilterHandler(stubFilterRepo{})(marshal(t, event.GetFilterEvent{UserId: "u", FilterId: "f"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		f := resp.(event.GetFilterResponse)
		if f.Id != "f" {
			t.Fatalf("expected filter id f, got %s", f.Id)
		}
	})
}

func TestStreamGetFiltersHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamGetFiltersHandler(stubFilterRepo{})(marshal(t, event.GetFiltersEvent{}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamGetFiltersHandler(stubFilterRepo{listFn: func(_ string, _ *uint64, _ *string) ([]domain.Filter, string, error) {
			return []domain.Filter{{Id: "a"}, {Id: "b"}}, "end", nil
		}})(marshal(t, event.GetFiltersEvent{UserId: "u"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		r := resp.(event.GetFiltersResponse)
		if len(r.Filters) != 2 {
			t.Fatalf("expected 2 filters, got %d", len(r.Filters))
		}
	})
}

func TestStreamNewFilterHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamNewFilterHandler(stubFilterRepo{})(marshal(t, event.NewFilterEvent{Title: "x"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty title", func(t *testing.T) {
		_, err := StreamNewFilterHandler(stubFilterRepo{})(marshal(t, event.NewFilterEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("create error", func(t *testing.T) {
		repoErr := errors.New("boom")
		_, err := StreamNewFilterHandler(stubFilterRepo{createFn: func(_ string, _ domain.Filter) (domain.Filter, error) {
			return domain.Filter{}, repoErr
		}})(marshal(t, event.NewFilterEvent{UserId: "u", Title: "t"}), nil)
		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repo error: %v", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamNewFilterHandler(stubFilterRepo{})(marshal(t, event.NewFilterEvent{UserId: "u", Title: "t"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		f := resp.(domain.Filter)
		if f.UserId != "u" || f.Title != "t" {
			t.Fatalf("bad filter: %+v", f)
		}
	})
}

func TestStreamUpdateFilterHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamUpdateFilterHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterEvent{Id: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty filter id", func(t *testing.T) {
		_, err := StreamUpdateFilterHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUpdateFilterHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterEvent{UserId: "u", Id: "f", Title: "new"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		f := resp.(domain.Filter)
		if f.Title != "new" {
			t.Fatalf("bad title: %s", f.Title)
		}
	})
}

func TestStreamDeleteFilterHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamDeleteFilterHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterEvent{FilterId: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty filter id", func(t *testing.T) {
		_, err := StreamDeleteFilterHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamDeleteFilterHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterEvent{UserId: "u", FilterId: "f"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}

func TestStreamAddFilterKeywordHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamAddFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.AddFilterKeywordEvent{FilterId: "f", Keyword: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty filter id", func(t *testing.T) {
		_, err := StreamAddFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.AddFilterKeywordEvent{UserId: "u", Keyword: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty keyword", func(t *testing.T) {
		_, err := StreamAddFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.AddFilterKeywordEvent{UserId: "u", FilterId: "f"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamAddFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.AddFilterKeywordEvent{
			UserId: "u", FilterId: "f", Keyword: "badword", WholeWord: true,
		}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		kw := resp.(domain.FilterKeyword)
		if kw.Keyword != "badword" || !kw.WholeWord {
			t.Fatalf("bad keyword: %+v", kw)
		}
	})
}

func TestStreamUpdateFilterKeywordHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamUpdateFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterKeywordEvent{KeywordId: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty keyword id", func(t *testing.T) {
		_, err := StreamUpdateFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterKeywordEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamUpdateFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.UpdateFilterKeywordEvent{
			UserId: "u", KeywordId: "k", Keyword: "new",
		}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		kw := resp.(domain.FilterKeyword)
		if kw.Id != "k" || kw.Keyword != "new" {
			t.Fatalf("bad keyword: %+v", kw)
		}
	})
}

func TestStreamDeleteFilterKeywordHandler(t *testing.T) {
	t.Run("empty user", func(t *testing.T) {
		_, err := StreamDeleteFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterKeywordEvent{KeywordId: "k"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty keyword id", func(t *testing.T) {
		_, err := StreamDeleteFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterKeywordEvent{UserId: "u"}), nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("happy path", func(t *testing.T) {
		resp, err := StreamDeleteFilterKeywordHandler(stubFilterRepo{})(marshal(t, event.DeleteFilterKeywordEvent{UserId: "u", KeywordId: "k"}), nil)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if resp != event.Accepted {
			t.Fatal("expected Accepted")
		}
	})
}
