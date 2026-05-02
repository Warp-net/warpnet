/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.components.search.paging

import androidx.paging.PagingSource
import androidx.paging.PagingState
import com.keylesspalace.tusky.viewdata.StatusViewData

class SearchStatusPagingSource(
    private val items: List<StatusViewData.Concrete>,
    private val nextKey: Int?
) : PagingSource<Int, StatusViewData.Concrete>() {

    override fun getRefreshKey(state: PagingState<Int, StatusViewData.Concrete>): Int? = null

    override suspend fun load(params: LoadParams<Int>): LoadResult<Int, StatusViewData.Concrete> {
        return if (params is LoadParams.Refresh) {
            LoadResult.Page(items.toList(), null, nextKey)
        } else {
            LoadResult.Page(emptyList(), null, null)
        }
    }
}
