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

import android.util.Log
import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import site.warpnet.warpdroid.components.timeline.util.ifExpected
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.util.HttpHeaderLink
import site.warpnet.warpdroid.util.toViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import retrofit2.HttpException

@OptIn(ExperimentalPagingApi::class)
class NetworkTimelineRemoteMediator(
    private val viewModel: NetworkTimelineViewModel
) : RemoteMediator<String, TweetViewData>() {

    override suspend fun load(
        loadType: LoadType,
        state: PagingState<String, TweetViewData>
    ): MediatorResult {
        try {
            val statusResponse = when (loadType) {
                LoadType.REFRESH -> {
                    viewModel.fetchStatusesForKind(limit = state.config.pageSize)
                }
                LoadType.PREPEND -> {
                    return MediatorResult.Success(endOfPaginationReached = true)
                }
                LoadType.APPEND -> {
                    val maxId = viewModel.nextKey
                    if (maxId != null) {
                        viewModel.fetchStatusesForKind(maxId = maxId, limit = state.config.pageSize)
                    } else {
                        return MediatorResult.Success(endOfPaginationReached = true)
                    }
                }
            }

            val statuses = statusResponse.body()
            if (!statusResponse.isSuccessful || statuses == null) {
                return MediatorResult.Error(HttpException(statusResponse))
            }

            if (!viewModel.kind.isOrdered && loadType == LoadType.REFRESH) {
                // Not an ordered timeline, so we must reload everything in order to avoid duplicate posts
                viewModel.statusData.clear()
            }

            // Same race as TimelineViewModel.init: if the account flow
            // hasn't propagated its first value yet on a fresh process,
            // fall back to the AccountManager's eagerly-computed active
            // account (always non-null thanks to the stub). Treat a fully
            // missing account as an empty page rather than crash.
            val activeAccount = viewModel.activeAccountFlow.value
                ?: viewModel.accountManager.activeAccount
                ?: return MediatorResult.Success(endOfPaginationReached = true)

            val data = statuses.map { status ->

                val oldStatus = viewModel.statusData.find { s ->
                    s.asStatusOrNull()?.id == status.id
                }?.asStatusOrNull()

                val oldQuoteViewData = oldStatus?.quote?.quotedTweetViewData

                val contentShowing = oldStatus?.isShowingContent
                    ?: status.shouldShowContent(activeAccount.alwaysShowSensitiveMedia, viewModel.kind.toFilterKind())
                val expanded = oldStatus?.isExpanded ?: activeAccount.alwaysOpenSpoiler
                val contentCollapsed = oldStatus?.isCollapsed != false
                val filterActive = oldStatus?.filterActive ?: true

                val isQuoteShowingContent = oldQuoteViewData?.isShowingContent
                    ?: status.quote?.quotedStatus?.shouldShowContent(activeAccount.alwaysShowSensitiveMedia, Filter.Kind.THREAD)
                    ?: activeAccount.alwaysShowSensitiveMedia
                val isQuoteExpanded = oldQuoteViewData?.isExpanded ?: activeAccount.alwaysOpenSpoiler
                val isQuoteCollapsed = oldQuoteViewData?.isCollapsed ?: true

                status.toViewData(
                    isShowingContent = contentShowing,
                    isExpanded = expanded,
                    isCollapsed = contentCollapsed,
                    filterKind = viewModel.kind.toFilterKind(),
                    filterActive = filterActive,
                    isQuoteShowingContent = isQuoteShowingContent,
                    isQuoteExpanded = isQuoteExpanded,
                    isQuoteCollapsed = isQuoteCollapsed,
                    isQuoteShown = oldStatus?.quote?.quoteShown ?: false
                )
            }

            if (loadType == LoadType.REFRESH && viewModel.statusData.isNotEmpty()) {
                val insertPlaceholder = if (statuses.isNotEmpty()) {
                    !viewModel.statusData.removeAll { statusViewData ->
                        statuses.any { status -> status.id == statusViewData.asStatusOrNull()?.id }
                    }
                } else {
                    false
                }

                viewModel.statusData.addAll(0, data)

                if (insertPlaceholder) {
                    viewModel.statusData[statuses.size - 1] = TweetViewData.LoadMore(statuses.last().id, false)
                }
            } else {
                val linkHeader = statusResponse.headers()["Link"]
                val links = HttpHeaderLink.parse(linkHeader)
                val next = HttpHeaderLink.findByRelationType(links, "next")

                viewModel.nextKey = next?.uri?.getQueryParameter("max_id")
                viewModel.statusData.addAll(data)
            }

            viewModel.currentSource?.invalidate()
            return MediatorResult.Success(endOfPaginationReached = statuses.isEmpty())
        } catch (e: Exception) {
            return ifExpected(e) {
                Log.w(TAG, "Failed to load timeline", e)
                MediatorResult.Error(e)
            }
        }
    }

    companion object {
        private const val TAG = "NetworkTimelineRM"
    }
}
