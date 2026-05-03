/* Copyright 2022 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.account.media

import androidx.paging.ExperimentalPagingApi
import androidx.paging.LoadType
import androidx.paging.PagingState
import androidx.paging.RemoteMediator
import site.warpnet.warpdroid.components.timeline.util.ifExpected
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import retrofit2.HttpException

@OptIn(ExperimentalPagingApi::class)
class AccountMediaRemoteMediator(
    private val api: WarpnetApi,
    private val activeAccount: AccountEntity,
    private val viewModel: AccountMediaViewModel
) : RemoteMediator<String, AttachmentViewData>() {
    override suspend fun load(
        loadType: LoadType,
        state: PagingState<String, AttachmentViewData>
    ): MediatorResult {
        try {
            val statusResponse = when (loadType) {
                LoadType.REFRESH -> {
                    api.accountStatuses(viewModel.accountId, onlyMedia = true)
                }
                LoadType.PREPEND -> {
                    return MediatorResult.Success(endOfPaginationReached = true)
                }
                LoadType.APPEND -> {
                    val maxId = state.lastItemOrNull()?.statusId
                    if (maxId != null) {
                        api.accountStatuses(accountId = viewModel.accountId, maxId = maxId, excludeReplies = null, onlyMedia = true)
                    } else {
                        return MediatorResult.Success(endOfPaginationReached = false)
                    }
                }
            }

            val statuses = statusResponse.body()
            if (!statusResponse.isSuccessful || statuses == null) {
                return MediatorResult.Error(HttpException(statusResponse))
            }

            val attachments = statuses.flatMap { status ->
                status.attachments.map { attachment ->
                    AttachmentViewData(
                        attachment = attachment,
                        statusId = status.id,
                        statusUrl = status.url.orEmpty(),
                        sensitive = status.sensitive,
                        isRevealed = activeAccount.alwaysShowSensitiveMedia || !status.sensitive
                    )
                }
            }

            if (loadType == LoadType.REFRESH) {
                viewModel.attachmentData.clear()
            }

            viewModel.attachmentData.addAll(attachments)

            viewModel.currentSource?.invalidate()
            return MediatorResult.Success(endOfPaginationReached = statuses.isEmpty())
        } catch (e: Exception) {
            return ifExpected(e) {
                MediatorResult.Error(e)
            }
        }
    }
}
