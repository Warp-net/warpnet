/*
 * Copyright 2023 Warpdroid Contributors
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
 * see <http://www.gnu.org/licenses>.
 */

package site.warpnet.warpdroid.worker

import android.app.Notification
import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import site.warpnet.transport.ConnectionState
import site.warpnet.transport.WarpnetClient
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.systemnotifications.NotificationFetcher
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

@HiltWorker
class NotificationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val notificationsFetcher: NotificationFetcher,
    notificationHelper: NotificationHelper,
    private val client: WarpnetClient,
) : CoroutineWorker(appContext, params) {
    val notification: Notification = notificationHelper.createWorkerNotification(
        R.string.notification_notification_worker
    )

    override suspend fun doWork(): Result {
        val accountId = inputData.getLong(KEY_ACCOUNT_ID, 0).takeIf { it != 0L }

        // Bringing the libp2p host up is the pairing entity's job
        // (PairRefreshWorker in the background, the foreground lifecycle
        // otherwise). Here we only ride the result of that pairing: pull when
        // the link is live, otherwise skip and let a later cycle catch up once
        // a connection exists.
        if (client.state.value !is ConnectionState.Connected) {
            return Result.success()
        }

        notificationsFetcher.fetchAndShow(accountId)
        return Result.success()
    }

    override suspend fun getForegroundInfo() = ForegroundInfo(
        NotificationHelper.NOTIFICATION_ID_FETCH_NOTIFICATION,
        notification
    )

    companion object {
        const val KEY_ACCOUNT_ID = "accountId"
    }
}
