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
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import site.warpnet.transport.ConnectionState
import site.warpnet.transport.WarpnetClient
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.pairing.PairingCoordinator
import site.warpnet.warpdroid.components.systemnotifications.NotificationFetcher
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

@HiltWorker
class NotificationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val notificationsFetcher: NotificationFetcher,
    notificationHelper: NotificationHelper,
    private val client: WarpnetClient,
    private val pairingCoordinator: PairingCoordinator,
) : CoroutineWorker(appContext, params) {
    val notification: Notification = notificationHelper.createWorkerNotification(
        R.string.notification_notification_worker
    )

    override suspend fun doWork(): Result {
        val accountId = inputData.getLong(KEY_ACCOUNT_ID, 0).takeIf { it != 0L }

        // The libp2p host is paused while the app is backgrounded, so the
        // periodic pull has no live connection to ride. Wake it ourselves
        // (the foreground path can't), then release it again so the relay
        // keep-alive doesn't drain the battery between polls. When the app is
        // foregrounded the UI owns the connection — don't touch it.
        val foreground = withContext(Dispatchers.Main) {
            ProcessLifecycleOwner.get().lifecycle.currentState
                .isAtLeast(Lifecycle.State.STARTED)
        }
        val wokeHost = !foreground && client.state.value !is ConnectionState.Connected
        if (wokeHost && !pairingCoordinator.ensureConnected()) {
            return Result.success() // not paired or node unreachable; retry next cycle
        }

        try {
            notificationsFetcher.fetchAndShow(accountId)
        } finally {
            if (wokeHost) {
                client.pause()
            }
        }
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
