/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.worker

import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.hilt.work.HiltWorker
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequest
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import site.warpnet.transport.WarpnetClient
import site.warpnet.transport.WarpnetException
import site.warpnet.warpdroid.components.pairing.ConnectionStatus
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.components.pairing.PairingActivity
import site.warpnet.warpdroid.components.pairing.isDurablePairRejection

/**
 * Periodically re-pings the fat node's /private/post/pair handler so the
 * peerstore picks up any fresh public addresses the fat node has gained
 * (e.g. after moving between networks). The pair response itself carries
 * the address list; [WarpnetClient.pair] merges it into the peerstore.
 */
@HiltWorker
class PairRefreshWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted params: WorkerParameters,
    private val client: WarpnetClient,
    private val pairedNodeStore: PairedNodeStore,
    private val connectionStatus: ConnectionStatus,
) : CoroutineWorker(appContext, params) {

    override suspend fun doWork(): Result {
        val rawQr = pairedNodeStore.loadRawQr() ?: run {
            // No pairing (none yet, or cleared by an unpair): stop reporting
            // connected so NotificationWorker doesn't keep trying to pull.
            connectionStatus.update(false)
            return Result.success()
        }
        return try {
            client.pair(rawQr)
            // Pair succeeded → connected. Publish it so NotificationWorker knows
            // a pull can reach the node this cycle.
            connectionStatus.update(true)
            Result.success()
        } catch (e: WarpnetException.NotConnected) {
            // App likely backgrounded; the host is paused and the next
            // foreground transition will redial. No point retrying with
            // backoff and burning the radio.
            connectionStatus.update(false)
            Log.d(TAG, "pair refresh skipped: not connected")
            Result.success()
        } catch (e: WarpnetException.NotInitialised) {
            connectionStatus.update(false)
            Log.d(TAG, "pair refresh skipped: not initialised")
            Result.success()
        } catch (e: WarpnetException.ProtocolError) {
            if (!e.isDurablePairRejection()) {
                Log.w(TAG, "pair refresh transient node error: code=${e.code} ${e.serverMessage}")
                Result.retry()
            } else {
                Log.w(TAG, "pair refresh rejected: code=${e.code} ${e.serverMessage}")
                connectionStatus.update(false)
                withContext(Dispatchers.IO) { pairedNodeStore.clear() }
                client.shutdown()
                val intent = Intent(applicationContext, PairingActivity::class.java)
                    .addFlags(
                        Intent.FLAG_ACTIVITY_NEW_TASK or
                            Intent.FLAG_ACTIVITY_CLEAR_TASK,
                    )
                applicationContext.startActivity(intent)
                Result.success()
            }
        } catch (e: Exception) {
            connectionStatus.update(false)
            Log.w(TAG, "pair refresh failed", e)
            Result.retry()
        }
    }

    companion object {
        private const val TAG = "PairRefreshWorker"
        const val UNIQUE_NAME = "pair-refresh"

        fun schedule(context: Context) {
            val request = PeriodicWorkRequest.Builder(
                PairRefreshWorker::class.java,
                PeriodicWorkRequest.MIN_PERIODIC_INTERVAL_MILLIS,
                TimeUnit.MILLISECONDS,
            )
                .setConstraints(
                    Constraints.Builder()
                        .setRequiredNetworkType(NetworkType.CONNECTED)
                        .setRequiresBatteryNotLow(true)
                        .build()
                )
                .setBackoffCriteria(
                    BackoffPolicy.LINEAR,
                    1L,
                    TimeUnit.MINUTES,
                )
                .build()

            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                UNIQUE_NAME,
                ExistingPeriodicWorkPolicy.KEEP,
                request,
            )
        }
    }
}
