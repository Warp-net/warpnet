/* Copyright 2024 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.notifications.requests.details

import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import site.warpnet.warpdroid.components.notifications.toViewData
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Quote
import site.warpnet.warpdroid.util.HttpHeaderLink
import site.warpnet.warpdroid.viewdata.NotificationViewData
import retrofit2.HttpException
import retrofit2.Response

@OptIn(ExperimentalPagingApi::class)
class NotificationRequestDetailsRemoteMediator(
    private val viewModel: NotificationRequestDetailsViewModel
) : RemoteMediator<String, NotificationViewData>() {

    override suspend fun load(
        loadType: LoadType,
        state: PagingState<String, NotificationViewData>
    ): MediatorResult {
        return try {
            val response = request(loadType)
                ?: return MediatorResult.Success(endOfPaginationReached = true)

            return applyResponse(response)
        } catch (e: Exception) {
            MediatorResult.Error(e)
        }
    }

    private suspend fun request(loadType: LoadType): Response<List<Notification>>? {
        return when (loadType) {
            LoadType.PREPEND -> null
            LoadType.APPEND -> viewModel.api.notifications(maxId = viewModel.nextKey, accountId = viewModel.accountId)
            LoadType.REFRESH -> {
                viewModel.nextKey = null
                viewModel.notificationData.clear()
                viewModel.api.notifications(accountId = viewModel.accountId)
            }
        }
    }

    private fun applyResponse(response: Response<List<Notification>>): MediatorResult {
        val notifications = response.body()
        if (!response.isSuccessful || notifications == null) {
            return MediatorResult.Error(HttpException(response))
        }

        val links = HttpHeaderLink.parse(response.headers()["Link"])
        viewModel.nextKey = HttpHeaderLink.findByRelationType(links, "next")?.uri?.getQueryParameter("max_id")

        val alwaysShowSensitiveMedia = viewModel.accountManager.activeAccount?.alwaysShowSensitiveMedia == true
        val alwaysOpenSpoiler = viewModel.accountManager.activeAccount?.alwaysOpenSpoiler == false
        val notificationData = notifications.map { notification ->
            notification.toViewData(
                isShowingContent = notification.status?.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.NOTIFICATIONS) ?: alwaysShowSensitiveMedia,
                isExpanded = alwaysOpenSpoiler,
                isCollapsed = true,
                filterKind = Filter.Kind.NOTIFICATIONS,
                isQuoteShowingContent = notification.status?.quote?.quotedStatus?.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.NOTIFICATIONS) ?: alwaysShowSensitiveMedia,
                isQuoteExpanded = alwaysOpenSpoiler,
                isQuoteCollapsed = true,
                isQuoteShown = notification.status?.quote?.state == Quote.State.ACCEPTED
            )
        }

        viewModel.notificationData.addAll(notificationData)
        viewModel.currentSource?.invalidate()

        return MediatorResult.Success(endOfPaginationReached = viewModel.nextKey == null)
    }
}
