/* Copyright 2025 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.search.paging

import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.components.search.SearchType
import site.warpnet.warpdroid.components.search.SearchViewModel
import site.warpnet.warpdroid.entity.SearchResult
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.viewdata.TweetViewData

@OptIn(ExperimentalPagingApi::class)
class SearchTweetRemoteMediator(
    private val api: WarpnetApi,
    private val searchRequest: String,
    private val onPageLoaded: (SearchResult) -> Boolean,
    private val currentOffset: () -> Int
) : RemoteMediator<Int, TweetViewData.Concrete>() {
    override suspend fun load(
        loadType: LoadType,
        state: PagingState<Int, TweetViewData.Concrete>
    ): MediatorResult {
        if (searchRequest.isBlank() || loadType == LoadType.PREPEND) {
            return MediatorResult.Success(endOfPaginationReached = true)
        }

        return api.search(
            query = searchRequest,
            type = SearchType.Tweet.apiParameter,
            resolve = true,
            limit = SearchViewModel.DEFAULT_LOAD_SIZE,
            offset = currentOffset(),
            following = false
        ).fold(
            onSuccess = { searchResult ->
                MediatorResult.Success(
                    endOfPaginationReached = onPageLoaded(searchResult)
                )
            },
            onFailure = { error ->
                MediatorResult.Error(error)
            }
        )
    }
}
