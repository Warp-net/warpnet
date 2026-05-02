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

import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import at.connyduck.calladapter.networkresult.fold
import com.keylesspalace.tusky.components.search.SearchType
import com.keylesspalace.tusky.components.search.SearchViewModel
import com.keylesspalace.tusky.entity.SearchResult
import com.keylesspalace.tusky.network.MastodonApi
import com.keylesspalace.tusky.viewdata.StatusViewData

@OptIn(ExperimentalPagingApi::class)
class SearchStatusRemoteMediator(
    private val api: MastodonApi,
    private val searchRequest: String,
    private val onPageLoaded: (SearchResult) -> Boolean,
    private val currentOffset: () -> Int
) : RemoteMediator<Int, StatusViewData.Concrete>() {
    override suspend fun load(
        loadType: LoadType,
        state: PagingState<Int, StatusViewData.Concrete>
    ): MediatorResult {
        if (searchRequest.isBlank() || loadType == LoadType.PREPEND) {
            return MediatorResult.Success(endOfPaginationReached = true)
        }

        return api.search(
            query = searchRequest,
            type = SearchType.Status.apiParameter,
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
