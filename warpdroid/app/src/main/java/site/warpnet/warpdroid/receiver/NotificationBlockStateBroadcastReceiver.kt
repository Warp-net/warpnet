/* Copyright 2022 Warpdroid contributors
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

package site.warpnet.warpdroid.receiver

import android.app.NotificationManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.di.ApplicationScope
import site.warpnet.warpdroid.network.WarpnetApi
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

@AndroidEntryPoint
class NotificationBlockStateBroadcastReceiver : BroadcastReceiver() {
    @Inject
    lateinit var warpnetApi: WarpnetApi

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var notificationHelper: NotificationHelper

    @Inject
    @ApplicationScope
    lateinit var externalScope: CoroutineScope

    override fun onReceive(context: Context, intent: Intent) {
        if (Build.VERSION.SDK_INT < 28) return
        if (!notificationHelper.arePushNotificationsAvailable()) return

        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

        val accountIdentifier = when (intent.action) {
            NotificationManager.ACTION_NOTIFICATION_CHANNEL_BLOCK_STATE_CHANGED -> {
                val channelId = intent.getStringExtra(NotificationManager.EXTRA_NOTIFICATION_CHANNEL_ID)
                nm.getNotificationChannel(channelId).group
            }
            NotificationManager.ACTION_NOTIFICATION_CHANNEL_GROUP_BLOCK_STATE_CHANGED -> {
                intent.getStringExtra(NotificationManager.EXTRA_NOTIFICATION_CHANNEL_GROUP_ID)
            }
            else -> null
        } ?: return

        accountManager.getAccountByIdentifier(accountIdentifier)?.let { account ->
            if (account.isPushNotificationsEnabled()) {
                externalScope.launch {
                    notificationHelper.updatePushSubscription(account)
                }
            }
        }
    }
}
