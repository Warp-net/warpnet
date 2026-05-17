/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
)

// FilterStorer is the narrow surface filter handlers need.
type FilterStorer interface {
	Create(userId string, f domain.Filter) (domain.Filter, error)
	Get(userId, filterId string) (domain.Filter, error)
	Update(userId string, f domain.Filter) (domain.Filter, error)
	Delete(userId, filterId string) error
	List(userId string, limit *uint64, cursor *string) ([]domain.Filter, string, error)
	AddKeyword(userId, filterId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
	UpdateKeyword(userId string, kw domain.FilterKeyword) (domain.FilterKeyword, error)
	DeleteKeyword(userId, keywordId string) error
}

func StreamGetFilterHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetFilterEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("get filter: empty user id")
		}
		if ev.FilterId == "" {
			return nil, warpnet.WarpError("get filter: empty filter id")
		}
		f, err := repo.Get(ev.UserId, ev.FilterId)
		if err != nil {
			return nil, err
		}
		return event.GetFilterResponse(f), nil
	}
}

func StreamGetFiltersHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.GetFiltersEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("get filters: empty user id")
		}
		fs, cur, err := repo.List(ev.UserId, ev.Limit, ev.Cursor)
		if err != nil {
			return nil, err
		}
		return event.GetFiltersResponse{Filters: fs, Cursor: cur}, nil
	}
}

func StreamNewFilterHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.NewFilterEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("new filter: empty user id")
		}
		if ev.Title == "" {
			return nil, warpnet.WarpError("new filter: empty title")
		}
		created, err := repo.Create(ev.UserId, ev)
		if err != nil {
			return nil, err
		}
		return created, nil
	}
}

func StreamUpdateFilterHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UpdateFilterEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("update filter: empty user id")
		}
		if ev.Id == "" {
			return nil, warpnet.WarpError("update filter: empty filter id")
		}
		updated, err := repo.Update(ev.UserId, ev)
		if err != nil {
			return nil, err
		}
		return updated, nil
	}
}

func StreamDeleteFilterHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteFilterEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("delete filter: empty user id")
		}
		if ev.FilterId == "" {
			return nil, warpnet.WarpError("delete filter: empty filter id")
		}
		if err := repo.Delete(ev.UserId, ev.FilterId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}

func StreamAddFilterKeywordHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.AddFilterKeywordEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("add filter keyword: empty user id")
		}
		if ev.FilterId == "" {
			return nil, warpnet.WarpError("add filter keyword: empty filter id")
		}
		if ev.Keyword == "" {
			return nil, warpnet.WarpError("add filter keyword: empty keyword")
		}
		kw, err := repo.AddKeyword(ev.UserId, ev.FilterId, domain.FilterKeyword{
			Keyword:   ev.Keyword,
			WholeWord: ev.WholeWord,
		})
		if err != nil {
			return nil, err
		}
		return kw, nil
	}
}

func StreamUpdateFilterKeywordHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.UpdateFilterKeywordEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("update filter keyword: empty user id")
		}
		if ev.KeywordId == "" {
			return nil, warpnet.WarpError("update filter keyword: empty keyword id")
		}
		kw, err := repo.UpdateKeyword(ev.UserId, domain.FilterKeyword{
			Id:        ev.KeywordId,
			Keyword:   ev.Keyword,
			WholeWord: ev.WholeWord,
		})
		if err != nil {
			return nil, err
		}
		return kw, nil
	}
}

func StreamDeleteFilterKeywordHandler(repo FilterStorer) warpnet.WarpHandlerFunc {
	return func(buf []byte, s warpnet.WarpStream) (any, error) {
		var ev event.DeleteFilterKeywordEvent
		if err := json.Unmarshal(buf, &ev); err != nil {
			return nil, err
		}
		if ev.UserId == "" {
			return nil, warpnet.WarpError("delete filter keyword: empty user id")
		}
		if ev.KeywordId == "" {
			return nil, warpnet.WarpError("delete filter keyword: empty keyword id")
		}
		if err := repo.DeleteKeyword(ev.UserId, ev.KeywordId); err != nil {
			return nil, err
		}
		return event.Accepted, nil
	}
}
