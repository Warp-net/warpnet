/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package site.warpnet.warpdroid.receiver

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.service.WarpnetNotificationService
import site.warpnet.warpdroid.settings.PrefKeys
import timber.log.Timber

/**
 * Restarts the Briar-style push service after a reboot or an app update, so
 * notifications keep arriving without the user having to open the app first.
 * The service is only started when the device is already paired and the user
 * hasn't turned notifications off.
 */
@AndroidEntryPoint
class BootReceiver : BroadcastReceiver() {

    @Inject
    lateinit var pairedNodeStore: PairedNodeStore

    @Inject
    lateinit var preferences: SharedPreferences

    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action
        if (action != Intent.ACTION_BOOT_COMPLETED &&
            action != Intent.ACTION_MY_PACKAGE_REPLACED
        ) {
            return
        }

        // loadRawQr touches the KeyStore + disk; keep the receiver alive while
        // we check off the main thread, then start the foreground service.
        val pending = goAsync()
        val appContext = context.applicationContext
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val paired = runCatching { pairedNodeStore.loadRawQr() }.getOrNull() != null
                val enabled = preferences.getBoolean(PrefKeys.NOTIFICATIONS_ENABLED, true)
                if (paired && enabled) {
                    runCatching { WarpnetNotificationService.start(appContext) }
                        .onFailure { Timber.tag(TAG).w(it, "failed to start push service on boot") }
                }
            } finally {
                pending.finish()
            }
        }
    }

    private companion object {
        const val TAG = "BootReceiver"
    }
}
