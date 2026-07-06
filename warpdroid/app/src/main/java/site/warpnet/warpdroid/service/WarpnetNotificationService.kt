/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package site.warpnet.warpdroid.service

import android.annotation.SuppressLint
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.squareup.moshi.Moshi
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import site.warpnet.transport.ConnectionState
import site.warpnet.transport.ConnectionMonitor
import site.warpnet.transport.WarpnetClient
import site.warpnet.warpdroid.components.pairing.AuthNodeInfoValidator
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.components.pairing.PairingActivity
import site.warpnet.warpdroid.components.pairing.PairingCoordinator
import site.warpnet.warpdroid.components.pairing.PairingOutcome
import site.warpnet.warpdroid.components.pairing.ValidationResult
import site.warpnet.warpdroid.components.systemnotifications.NotificationFetcher
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import timber.log.Timber

// Briar-style push: keeps the libp2p node connected in the background and
// delta-polls the fat node's push queue, posting local notifications. Warpnet
// has no cloud push gateway (no FCM).
@AndroidEntryPoint
class WarpnetNotificationService : Service() {

    @Inject
    lateinit var warpnetClient: WarpnetClient

    @Inject
    lateinit var connectionMonitor: ConnectionMonitor

    @Inject
    lateinit var notificationFetcher: NotificationFetcher

    @Inject
    lateinit var notificationHelper: NotificationHelper

    @Inject
    lateinit var pairedNodeStore: PairedNodeStore

    @Inject
    lateinit var pairingCoordinator: PairingCoordinator

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var moshi: Moshi

    private val validator by lazy { AuthNodeInfoValidator(moshi) }

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var loopJob: Job? = null
    private var wakeLock: PowerManager.WakeLock? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        acquireWakeLock()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        isRunning = true // also set here to cover OS-driven START_STICKY restarts
        startAsForeground()
        if (loopJob?.isActive != true) {
            loopJob = scope.launch { runLoop() }
        }
        return START_STICKY
    }

    // Keeps the CPU awake while the screen is off so the poll loop and libp2p
    // keep-alive don't stall. Released in onDestroy.
    @SuppressLint("WakelockTimeout")
    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) return
        val powerManager = getSystemService(Context.POWER_SERVICE) as? PowerManager ?: return
        val lock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, WAKE_LOCK_TAG)
        lock.setReferenceCounted(false)
        runCatching { lock.acquire() }
            .onFailure { Timber.tag(TAG).w(it, "could not acquire wake lock") }
        wakeLock = lock
    }

    private fun releaseWakeLock() {
        wakeLock?.let { lock ->
            if (lock.isHeld) runCatching { lock.release() }
        }
        wakeLock = null
    }

    private fun startAsForeground() {
        val notification = notificationHelper.createSyncServiceNotification()
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
        } else {
            0
        }
        ServiceCompat.startForeground(
            this,
            NotificationHelper.NOTIFICATION_ID_SYNC_SERVICE,
            notification,
            type,
        )
    }

    private suspend fun runLoop() {
        // On a cold boot MainActivity hasn't created the channels yet, and
        // filterNotification() gates on them for API >= O.
        runCatching {
            accountManager.accounts.forEach { notificationHelper.createNotificationChannelsForAccount(it) }
        }

        ensureNodeUp()
        runCatching { connectionMonitor.start() }

        while (scope.isActive) {
            try {
                when (warpnetClient.state.value) {
                    is ConnectionState.Uninitialised -> ensureNodeUp()
                    is ConnectionState.Connected -> notificationFetcher.fetchAndShow(null)
                    else -> warpnetClient.resume()
                }
            } catch (ce: CancellationException) {
                throw ce
            } catch (e: Exception) {
                Timber.tag(TAG).w(e, "notification poll failed")
            }
            delay(POLL_INTERVAL_MS)
        }
    }

    // Warm start: just resume. Cold start (reboot): re-run the silent
    // auto-pair PairingActivity uses, from the keystore-backed QR payload.
    private suspend fun ensureNodeUp() {
        when (warpnetClient.state.value) {
            is ConnectionState.Uninitialised -> coldBringUp()
            else -> runCatching { warpnetClient.resume() }
        }
    }

    private suspend fun coldBringUp() {
        val rawQr = runCatching { pairedNodeStore.loadRawQr() }.getOrNull() ?: return
        when (val result = validator.validate(rawQr)) {
            is ValidationResult.Valid -> {
                val outcome = runCatching {
                    pairingCoordinator.pair(result.authNodeInfo, result.rawJson)
                }.getOrElse {
                    Timber.tag(TAG).w(it, "cold bring-up failed")
                    return
                }
                if (outcome is PairingOutcome.Rejected) {
                    Timber.tag(TAG).w(
                        "cold bring-up rejected (code=${outcome.code} ${outcome.message}); resetting pairing"
                    )
                    resetPairing()
                }
            }
            is ValidationResult.Invalid ->
                Timber.tag(TAG).w("cold bring-up: stored pairing invalid: ${result.reason}")
        }
    }

    private suspend fun resetPairing() {
        pairedNodeStore.clear()
        runCatching { warpnetClient.shutdown() }
        val intent = Intent(applicationContext, PairingActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
        applicationContext.startActivity(intent)
    }

    override fun onDestroy() {
        // isRunning is owned by start()/stop(), not cleared here, so an
        // OS-driven restart doesn't briefly report the service as stopped.
        scope.cancel()
        releaseWakeLock()
        super.onDestroy()
    }

    companion object {
        private const val TAG = "WarpnetNotifSvc"
        private const val WAKE_LOCK_TAG = "warpnet:push-service"
        private const val POLL_INTERVAL_MS = 30_000L

        // Intent-to-run, toggled at the control points so onStop can't race a
        // not-yet-created service.
        @Volatile
        var isRunning: Boolean = false
            private set

        fun start(context: Context) {
            isRunning = true
            val intent = Intent(context, WarpnetNotificationService::class.java)
            ContextCompat.startForegroundService(context, intent)
        }

        fun stop(context: Context) {
            isRunning = false
            context.stopService(Intent(context, WarpnetNotificationService::class.java))
        }
    }
}
