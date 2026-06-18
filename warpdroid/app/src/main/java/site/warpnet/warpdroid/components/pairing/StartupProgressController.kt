/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import android.widget.TextView
import androidx.annotation.StringRes
import com.google.android.material.progressindicator.LinearProgressIndicator
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * Drives the cold-start progress bar shown while the libp2p node initialises
 * and pairs (see [PairingActivity]).
 *
 * libp2p host start + Noise handshake + pairing has no fine-grained progress
 * callbacks, and on a fresh process it can take several seconds. An
 * indeterminate spinner over that window reads as "frozen", so instead we
 * animate a determinate bar 0 → [CAP_PERCENT]% over [fillDurationMs], stepping
 * [stageCaptions] through the node lifecycle (start → discover → handshake →
 * pair → connected) and the first tweet-load (profile → timeline).
 *
 * The timing is illustrative ("fake"), but completion is real: the bar holds
 * at [CAP_PERCENT]% until [complete] is called on the actual
 * PairingOutcome.Success, only then snapping to 100%. So the UI never claims
 * "done" before the node is genuinely connected, and a slow link just holds
 * near the end instead of looping forever.
 */
class StartupProgressController(
    private val indicator: LinearProgressIndicator,
    private val caption: TextView,
    @StringRes private val stageCaptions: List<Int>,
    private val fillDurationMs: Long = DEFAULT_FILL_MS,
) {
    @Volatile private var completed = false
    private var job: Job? = null

    /** Begin the timed fill + caption stepping on [scope] (main dispatcher). */
    fun start(scope: CoroutineScope) {
        completed = false
        indicator.isIndeterminate = false
        indicator.max = 100
        indicator.progress = 0
        job = scope.launch {
            val totalTicks = (fillDurationMs / TICK_MS).toInt().coerceAtLeast(1)
            var lastStage = -1
            for (tick in 1..totalTicks) {
                if (completed || !isActive) return@launch
                val fraction = tick.toFloat() / totalTicks
                indicator.progress = (fraction * CAP_PERCENT).toInt()
                val stage = (fraction * stageCaptions.size).toInt()
                    .coerceIn(0, stageCaptions.size - 1)
                if (stage != lastStage) {
                    caption.setText(stageCaptions[stage])
                    lastStage = stage
                }
                delay(TICK_MS)
            }
            // Reached the cap before pairing finished: hold here with the last
            // caption until complete() fires.
        }
    }

    /** Snap to 100% on real readiness (PairingOutcome.Success). */
    suspend fun complete(@StringRes finalCaption: Int) {
        completed = true
        job?.cancel()
        job = null
        caption.setText(finalCaption)
        indicator.setProgressCompat(100, true)
        delay(FINISH_HOLD_MS)
    }

    /** Stop the animation without finishing (pairing failed / will retry). */
    fun cancel() {
        completed = true
        job?.cancel()
        job = null
    }

    private companion object {
        const val DEFAULT_FILL_MS = 10_000L
        const val TICK_MS = 50L
        const val CAP_PERCENT = 90
        const val FINISH_HOLD_MS = 300L
    }
}
