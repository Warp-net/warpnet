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

package local

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
)

type (
	Key                  = ds.Key
	Batch                = ds.Batch
	Batching             = ds.Batching
	Filter               = dsq.Filter
	DsEntry              = dsq.Entry
	Query                = dsq.Query
	Results              = dsq.Results
	Result               = dsq.Result
	OrderByKey           = dsq.OrderByKey
	OrderByKeyDescending = dsq.OrderByKeyDescending
)

func NewKey(s string) ds.Key {
	return ds.NewKey(s)
}

func ResultsReplaceQuery(r Results, q Query) Results {
	return dsq.ResultsReplaceQuery(r, q)
}

func NaiveQueryApply(q Query, qr Results) Results {
	return dsq.NaiveQueryApply(q, qr)
}

func ResultsWithContext(q Query, proc func(context.Context, chan<- Result)) Results {
	return dsq.ResultsWithContext(q, proc)
}
