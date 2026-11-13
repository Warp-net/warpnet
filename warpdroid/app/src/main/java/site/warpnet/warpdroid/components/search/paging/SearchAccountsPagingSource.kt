/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.search.paging

import androidx.paging.PagingSource
import androidx.paging.PagingState
import kotlinx.coroutines.CancellationException
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.network.WarpnetApi

/** User search keyed by the node's opaque cursor; an empty key is the first page. */
class SearchAccountsPagingSource(
    private val warpnetApi: WarpnetApi,
    private val searchRequest: String,
) : PagingSource<String, TimelineUser>() {

    override fun getRefreshKey(state: PagingState<String, TimelineUser>): String? = null

    override suspend fun load(params: LoadParams<String>): LoadResult<String, TimelineUser> {
        val query = searchRequest.trim()
        if (query.isEmpty()) return LoadResult.Page(emptyList(), prevKey = null, nextKey = null)
        return try {
            val (users, next) = warpnetApi.searchAccountsPage(
                query = query,
                cursor = params.key.orEmpty(),
                limit = params.loadSize,
                resolve = true,
            )
            LoadResult.Page(users, prevKey = null, nextKey = next.ifEmpty { null })
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            LoadResult.Error(e)
        }
    }
}
