/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.logviewer

import android.util.Log
import site.warpnet.transport.NodeLogSink
import timber.log.Timber

/**
 * Timber tree that mirrors Kotlin-side logs into the [LogBuffer] so the
 * Logs screen shows both node (Go) and app (Kotlin) output. Planted in
 * WarpdroidApplication next to the debug-build [Timber.DebugTree].
 */
class LogBufferTree(private val buffer: LogBuffer) : Timber.Tree() {

    override fun log(priority: Int, tag: String?, message: String, t: Throwable?) {
        val level = when (priority) {
            Log.VERBOSE, Log.DEBUG -> NodeLogSink.LEVEL_DEBUG
            Log.INFO -> NodeLogSink.LEVEL_INFO
            Log.WARN -> NodeLogSink.LEVEL_WARN
            else -> NodeLogSink.LEVEL_ERROR
        }
        buffer.add(level, LogBuffer.Source.KOTLIN, tag ?: "app", message)
    }
}
