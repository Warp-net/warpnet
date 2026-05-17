/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * UI-facing transport state. Every screen that depends on the desktop node
 * observes [WarpnetClient.state] and renders an offline banner for any state
 * other than [Connected].
 */
sealed class ConnectionState {
    /** Binding has not been initialised yet (PSK + bootstrap not supplied). */
    data object Uninitialised : ConnectionState()

    /** Binding initialised but no desktop peer connected yet. */
    data object Disconnected : ConnectionState()

    /** Currently attempting to connect to the desktop peer. */
    data object Connecting : ConnectionState()

    /** Stream to desktop peer is open. */
    data object Connected : ConnectionState()

    /** Transport errored; caller can retry. */
    data class Failed(val error: WarpnetException) : ConnectionState()
}
