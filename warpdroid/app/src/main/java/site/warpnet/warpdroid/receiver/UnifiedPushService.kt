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

import android.util.Log
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.di.ApplicationScope
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import org.unifiedpush.android.connector.FailedReason
import org.unifiedpush.android.connector.PushService
import org.unifiedpush.android.connector.data.PushEndpoint
import org.unifiedpush.android.connector.data.PushMessage

@AndroidEntryPoint
class UnifiedPushService : PushService() {
    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var notificationHelper: NotificationHelper

    @Inject
    @ApplicationScope
    lateinit var applicationScope: CoroutineScope

    override fun onMessage(message: PushMessage, instance: String) {
        Log.d(TAG, "New message received for account $instance: $message")
        accountManager.getAccountById(instance.toLong())?.let { account ->
            notificationHelper.fetchNotificationsOnPushMessage(account)
        }
    }

    override fun onNewEndpoint(endpoint: PushEndpoint, instance: String) {
        Log.d(TAG, "Endpoint available for account $instance: $endpoint")
        accountManager.getAccountById(instance.toLong())?.let { account ->
            applicationScope.launch { notificationHelper.registerPushEndpoint(account, endpoint) }
        }
    }

    override fun onRegistrationFailed(reason: FailedReason, instance: String) = Unit

    override fun onUnregistered(instance: String) {
        Log.d(TAG, "Endpoint unregistered for account $instance")
        accountManager.getAccountById(instance.toLong())?.let { account ->
            // It's fine if the account does not exist anymore -- that means it has been logged out,
            // which removes the subscription anyway
            applicationScope.launch { notificationHelper.unregisterPushEndpoint(account) }
        }
    }

    companion object {
        const val TAG = "UnifiedPushService"
    }
}
