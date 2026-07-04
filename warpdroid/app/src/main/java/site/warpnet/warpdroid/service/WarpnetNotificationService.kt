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
import site.warpnet.warpdroid.components.pairing.PairingCoordinator
import site.warpnet.warpdroid.components.pairing.ValidationResult
import site.warpnet.warpdroid.components.systemnotifications.NotificationFetcher
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import timber.log.Timber

/**
 * Briar-style push service: an ongoing foreground service that keeps the
 * embedded libp2p node connected to the paired fat node while the app is
 * backgrounded and delta-polls the fat node's push queue, posting local
 * Android notifications as new events arrive.
 *
 * Warpnet has no cloud push gateway (no FCM). The thin client is a
 * `NoListenAddrs` libp2p node, so the fat node can't open a stream back to
 * it — real-time delivery instead relies on this service holding the
 * connection open (mirroring how Briar keeps its transport stack alive) and
 * pulling [NotificationHelper]/[NotificationFetcher] on a short interval.
 *
 * The service is started from [NotificationHelper.enablePullNotifications]
 * (app start / notifications toggled on) and after a reboot by
 * [site.warpnet.warpdroid.receiver.BootReceiver]. START_STICKY plus the boot
 * receiver give Briar's "always running until the user turns it off"
 * behaviour.
 */
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
        // Set here too (not only in start()) so an OS-driven START_STICKY
        // restart also marks the service as running.
        isRunning = true
        startAsForeground()
        if (loopJob?.isActive != true) {
            loopJob = scope.launch { runLoop() }
        }
        // Briar keeps the service alive until the user signs out; START_STICKY
        // asks the OS to recreate it if it's killed for memory.
        return START_STICKY
    }

    /**
     * Hold a partial wake lock for the lifetime of the always-on push
     * service. Without it the CPU can suspend while the screen is off, which
     * pauses the poll loop's timer and the libp2p keep-alive, so notifications
     * would only arrive when the device happens to wake for something else.
     * Released in [onDestroy]; the app is expected to be battery-optimization
     * exempt (requested in MainActivity) so this is allowed to run.
     */
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
        // Make sure the per-account channels exist before the first fetch —
        // on a cold boot MainActivity hasn't run, so channel creation (which
        // filterNotification() gates on for API >= O) hasn't happened yet.
        runCatching {
            accountManager.accounts.forEach { notificationHelper.createNotificationChannelsForAccount(it) }
        }

        ensureNodeUp()
        // The monitor owns reconnect/backoff; starting it here keeps the link
        // alive while the app is backgrounded and the service is holding it.
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

    /**
     * Bring the embedded node up if it isn't already. On a warm start
     * (app was open) the host is initialised and only needs a resume; on a
     * cold start (reboot) it re-runs the silent auto-pair that
     * PairingActivity uses on launch, using the keystore-backed QR payload.
     */
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
                runCatching { pairingCoordinator.pair(result.authNodeInfo, result.rawJson) }
                    .onFailure { Timber.tag(TAG).w(it, "cold bring-up failed") }
            }
            is ValidationResult.Invalid ->
                Timber.tag(TAG).w("cold bring-up: stored pairing invalid: ${result.reason}")
        }
    }

    override fun onDestroy() {
        // Don't clear isRunning here: a START_STICKY restart or user stop()
        // owns that flag. Clearing it on every teardown would momentarily lie
        // to WarpdroidApplication.onStop during an OS-driven restart.
        scope.cancel()
        releaseWakeLock()
        super.onDestroy()
    }

    companion object {
        private const val TAG = "WarpnetNotifSvc"
        private const val WAKE_LOCK_TAG = "warpnet:push-service"

        // Delta-poll cadence while the connection is held open. Short enough
        // to feel like push, long enough not to hammer the radio.
        private const val POLL_INTERVAL_MS = 30_000L

        // Reflects intent-to-run, toggled at the control points (start/stop)
        // rather than in the async onCreate/onDestroy lifecycle, so
        // WarpdroidApplication.onStop never races a not-yet-created service.
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
