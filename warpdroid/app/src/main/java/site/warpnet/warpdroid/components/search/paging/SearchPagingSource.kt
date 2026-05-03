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

package site.warpnet.warpdroid.components.search.paging

import androidx.paging.PagingSource
import androidx.paging.PagingState
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.components.search.SearchType
import site.warpnet.warpdroid.entity.SearchResult
import site.warpnet.warpdroid.network.WarpnetApi

/** for account and hashtag search */
class SearchPagingSource<T : Any>(
    private val warpnetApi: WarpnetApi,
    private val searchType: SearchType,
    private val searchRequest: String,
    private val parser: (SearchResult) -> List<T>
) : PagingSource<Int, T>() {

    override fun getRefreshKey(state: PagingState<Int, T>): Int? {
        return null
    }

    override suspend fun load(params: LoadParams<Int>): LoadResult<Int, T> {
        if (searchRequest.isEmpty()) {
            return LoadResult.Page(
                data = emptyList(),
                prevKey = null,
                nextKey = null
            )
        }

        val currentKey = params.key ?: 0

        return warpnetApi.search(
            query = searchRequest,
            type = searchType.apiParameter,
            resolve = true,
            limit = params.loadSize,
            offset = currentKey,
            following = false
        ).fold(
            onSuccess = { searchResult ->
                val res = parser(searchResult)

                val nextKey = if (res.isEmpty()) {
                    null
                } else {
                    currentKey + params.loadSize
                }

                return LoadResult.Page(
                    data = res,
                    prevKey = null,
                    nextKey = nextKey
                )
            },
            onFailure = { e ->
                LoadResult.Error(e)
            }
        )
    }
}
