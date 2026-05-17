/* Copyright 2021 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.components.timeline.viewmodel

import androidx.paging.PagingSource
import androidx.paging.PagingState
import site.warpnet.warpdroid.viewdata.TweetViewData

class NetworkTimelinePagingSource(
    private val viewModel: NetworkTimelineViewModel
) : PagingSource<String, TweetViewData>() {

    override fun getRefreshKey(state: PagingState<String, TweetViewData>): String? = null

    override suspend fun load(params: LoadParams<String>): LoadResult<String, TweetViewData> {
        return if (params is LoadParams.Refresh) {
            val list = viewModel.statusData.toList()
            LoadResult.Page(list, null, viewModel.nextKey)
        } else {
            LoadResult.Page(emptyList(), null, null)
        }
    }
}
