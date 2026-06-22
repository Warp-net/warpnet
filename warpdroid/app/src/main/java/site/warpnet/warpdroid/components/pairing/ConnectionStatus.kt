/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import javax.inject.Inject
import javax.inject.Singleton

/**
 * Last known link status to the paired fat node.
 *
 * Published by [site.warpnet.warpdroid.worker.PairRefreshWorker] — the only
 * entity that pairs — from the result of its pair handshake, and read by
 * [site.warpnet.warpdroid.worker.NotificationWorker] so the pull skips when
 * there is no live link instead of failing the fetch. Process-scoped: a cold
 * start defaults to not-connected until the next pair refresh reports in.
 */
@Singleton
class ConnectionStatus @Inject constructor() {
    @Volatile
    var isConnected: Boolean = false
        private set

    fun update(connected: Boolean) {
        isConnected = connected
    }
}
