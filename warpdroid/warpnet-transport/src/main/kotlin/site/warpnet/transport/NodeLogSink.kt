/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * Receives structured log lines from the embedded Go node. Registered
 * through [WarpnetBinding.setLogSink]; the gomobile side calls [write]
 * from arbitrary Go goroutines, so implementations must be thread-safe
 * and must not block — a slow sink stalls the node's logging path.
 *
 * Levels mirror `warpdroid/node/logsink.go`.
 */
fun interface NodeLogSink {
    fun write(level: Int, component: String, msg: String)

    companion object {
        const val LEVEL_DEBUG = 0
        const val LEVEL_INFO = 1
        const val LEVEL_WARN = 2
        const val LEVEL_ERROR = 3
    }
}
