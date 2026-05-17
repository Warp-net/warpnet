/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * Snapshot of the libp2p link to the paired desktop node, polled out of
 * the Go binding by [ConnectionMonitor]. Distinct from
 * [ConnectionState] (which tracks WarpnetClient lifecycle —
 * Uninitialised / Disconnected / Connecting / Connected / Failed) so the
 * UI can combine both: WarpnetClient.state for "are we paired?",
 * ConnectionMonitor.linkState for "is the wire up right now?".
 *
 * [Connected] and [Limited] are both "the wire is up" — Limited just
 * means traffic goes via a circuit-v2 relay. UI may want to badge it.
 * [NotConnected] is the only state that triggers the reconnect loop.
 * [Reconnecting] is purely cosmetic so the UI can show progress while
 * the monitor walks its candidate addresses.
 */
sealed class LinkState {
    data object Unknown : LinkState()
    data object Connected : LinkState()
    data object Limited : LinkState()
    data object NotConnected : LinkState()
    data class Reconnecting(val attempt: Int) : LinkState()

    val isUp: Boolean
        get() = this is Connected || this is Limited

    companion object {
        // Mirrors network.Connectedness#String() values emitted by the
        // Go binding's Connectedness() export. "CanConnect" and
        // "CannotConnect" are libp2p hints — treat as NotConnected for
        // our purposes since neither state has an open stream.
        fun fromBinding(raw: String): LinkState = when (raw) {
            "Connected" -> Connected
            "Limited" -> Limited
            "NotConnected", "CanConnect", "CannotConnect" -> NotConnected
            else -> Unknown
        }
    }
}
