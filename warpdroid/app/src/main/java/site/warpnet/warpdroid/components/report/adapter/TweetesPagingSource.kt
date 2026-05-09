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

package site.warpnet.warpdroid.components.report.adapter

import android.util.Log
import androidx.paging.PagingSource
import androidx.paging.PagingState
import at.connyduck.calladapter.networkresult.getOrThrow
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import retrofit2.HttpException

class StatusesPagingSource(
    private val accountId: String,
    private val warpnetApi: WarpnetApi
) : PagingSource<String, Tweet>() {

    override fun getRefreshKey(state: PagingState<String, Tweet>): String? {
        return state.anchorPosition?.let { anchorPosition ->
            state.closestItemToPosition(anchorPosition)?.id
        }
    }

    override suspend fun load(params: LoadParams<String>): LoadResult<String, Tweet> {
        val key = params.key
        try {
            val result = if (params is LoadParams.Refresh && key != null) {
                // Use coroutineScope to ensure that one failed call will cancel the other one
                // and the source Exception will be propagated locally.
                coroutineScope {
                    val initialStatus = async { getSingleStatus(key) }
                    val additionalStatuses =
                        async { getStatusList(maxId = key, limit = params.loadSize - 1) }
                    listOf(initialStatus.await()) + additionalStatuses.await()
                }
            } else {
                val maxId = if (params is LoadParams.Refresh || params is LoadParams.Append) {
                    params.key
                } else {
                    null
                }

                val minId = if (params is LoadParams.Prepend) {
                    params.key
                } else {
                    null
                }

                getStatusList(minId = minId, maxId = maxId, limit = params.loadSize)
            }
            return LoadResult.Page(
                data = result,
                prevKey = result.firstOrNull()?.id,
                nextKey = result.lastOrNull()?.id
            )
        } catch (e: Exception) {
            Log.w("StatusesPagingSource", "failed to load statuses", e)
            return LoadResult.Error(e)
        }
    }

    private suspend fun getSingleStatus(statusId: String): Tweet {
        return warpnetApi.status(statusId).getOrThrow()
    }

    private suspend fun getStatusList(
        minId: String? = null,
        maxId: String? = null,
        limit: Int
    ): List<Tweet> {
        val response = warpnetApi.accountStatuses(
            accountId = accountId,
            maxId = maxId,
            sinceId = null,
            minId = minId,
            limit = limit,
            excludeRetweets = true
        )
        val responseBody = response.body()
        if (response.isSuccessful && responseBody != null) {
            return responseBody
        } else {
            throw HttpException(response)
        }
    }
}
