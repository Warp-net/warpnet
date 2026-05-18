/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Polls the Go binding for the libp2p link state to the paired desktop
 * and re-dials when the connection drops. Owns reconnect policy and
 * backoff; the Go side stays a thin transport and only reports the
 * current snapshot. This is the Kotlin-owned counterpart of what would
 * otherwise be a network.Notifiee callback on the Go side.
 */
package site.warpnet.transport

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * @param client          live WarpnetClient; the monitor calls connect()
 *                        on it to drive reconnect attempts.
 * @param binding         the binding to poll; injected so unit tests can
 *                        feed a deterministic sequence of link states.
 * @param dialAddresses   suspending provider of currently-known dial
 *                        candidates. Kept as a lambda to avoid pulling
 *                        PairedNodeStore (which lives in :app) into
 *                        :warpnet-transport. App layer wires the lookup.
 * @param scope           parent scope; typically applicationScope so the
 *                        monitor outlives any one Activity but dies with
 *                        the process.
 */
class ConnectionMonitor(
    private val client: WarpnetClient,
    private val binding: WarpnetBinding,
    private val dialAddresses: suspend () -> List<String>,
    parentScope: CoroutineScope,
    private val pollIntervalMs: Long = 2_000L,
) {
    private val scope = CoroutineScope(parentScope.coroutineContext + SupervisorJob())

    private val _linkState = MutableStateFlow<LinkState>(LinkState.Unknown)
    val linkState: StateFlow<LinkState> = _linkState.asStateFlow()

    // Pairs of (start, stop) are serialised — restart on top of a live
    // monitor is a no-op rather than a double-loop, and shutdown waits
    // for the poller to actually exit before returning.
    private val controlMutex = Mutex()
    private var pollJob: Job? = null
    private var reconnectJob: Job? = null

    suspend fun start() = controlMutex.withLock {
        if (pollJob?.isActive == true) return@withLock
        pollJob = scope.launch(Dispatchers.IO) { pollLoop() }
    }

    suspend fun stop() = controlMutex.withLock {
        pollJob?.cancel()
        pollJob = null
        reconnectJob?.cancel()
        reconnectJob = null
        _linkState.value = LinkState.Unknown
    }

    fun shutdown() {
        scope.cancel()
    }

    private suspend fun pollLoop() {
        while (scope.isActive) {
            // While the client itself is uninitialised (no pairing yet,
            // or shutdown in flight) keep polling but don't attempt
            // reconnect — there's nothing to reconnect to.
            if (client.state.value == ConnectionState.Uninitialised) {
                publish(LinkState.Unknown)
                delay(pollIntervalMs)
                continue
            }
            val raw = runCatching { binding.connectedness() }
                .getOrDefault("NotConnected")
            val next = LinkState.fromBinding(raw)
            // Don't clobber a Reconnecting state with a transient
            // NotConnected poll — the reconnect loop itself owns those
            // transitions and emits Connected/Limited on success.
            val current = _linkState.value
            val skip = current is LinkState.Reconnecting && next is LinkState.NotConnected
            if (!skip) {
                publish(next)
            }
            // Kick off the reconnect loop only on the edge into
            // NotConnected from a previously-up state; subsequent polls
            // see Reconnecting and don't re-launch.
            if (next is LinkState.NotConnected &&
                current !is LinkState.Reconnecting &&
                reconnectJob?.isActive != true
            ) {
                reconnectJob = scope.launch(Dispatchers.IO) { reconnectLoop() }
            }
            delay(pollIntervalMs)
        }
    }

    private suspend fun reconnectLoop() {
        val backoffs = RECONNECT_BACKOFFS_MS
        var attempt = 0
        while (scope.isActive) {
            attempt++
            publish(LinkState.Reconnecting(attempt))
            val candidates = runCatching { dialAddresses() }.getOrDefault(emptyList())
            if (candidates.isEmpty()) {
                // Nothing to dial — bail and let the next poll surface
                // the steady-state NotConnected so the UI can show it.
                publish(LinkState.NotConnected)
                return
            }
            val outcome = runCatching { client.connect(candidates) }
            if (outcome.isSuccess) {
                // Let the next poll cycle observe the binding-level
                // state and emit Connected/Limited. We don't publish it
                // ourselves because connect returns the seeded addr, not
                // the connectedness — only the binding knows whether the
                // resulting connection is direct or relayed.
                return
            }
            val wait = backoffs.getOrElse(attempt - 1) { backoffs.last() }
            delay(wait)
        }
    }

    private fun publish(state: LinkState) {
        if (_linkState.value != state) {
            _linkState.value = state
        }
    }

    private companion object {
        // Reconnect cadence (ms): 1s, 2s, 5s, 10s, then 30s steady
        // state. Caps at 30s so we keep poking the peer even on long
        // outages without burning the radio.
        val RECONNECT_BACKOFFS_MS = longArrayOf(1_000L, 2_000L, 5_000L, 10_000L, 30_000L)
    }
}
