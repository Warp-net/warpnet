/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.logviewer

import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import site.warpnet.transport.NodeLogSink

/**
 * In-memory ring buffer behind the Logs screen. Fed from two sides:
 * the embedded Go node via [goSink] (called from Go goroutines through
 * JNI) and Kotlin code via [LogBufferTree]. Bounded at [CAPACITY]
 * entries — old lines are evicted, nothing is persisted.
 */
@Singleton
class LogBuffer @Inject constructor() {

    enum class Source { GO, KOTLIN }

    data class Entry(
        val timeMs: Long,
        val level: Int,
        val source: Source,
        val component: String,
        val message: String,
    )

    private val entries = ArrayDeque<Entry>()

    private val _updates = MutableSharedFlow<Entry>(
        extraBufferCapacity = 512,
        onBufferOverflow = BufferOverflow.DROP_OLDEST,
    )

    /** Hot stream of appended entries; pair with [snapshot] for history. */
    val updates: SharedFlow<Entry> = _updates

    /** Adapter handed to the Go node via WarpnetClient/WarpnetBinding. */
    val goSink: NodeLogSink = NodeLogSink { level, component, msg ->
        add(level, Source.GO, component, msg)
    }

    fun add(level: Int, source: Source, component: String, message: String) {
        val entry = Entry(
            timeMs = System.currentTimeMillis(),
            level = level.coerceIn(NodeLogSink.LEVEL_DEBUG, NodeLogSink.LEVEL_ERROR),
            source = source,
            component = component,
            message = message,
        )
        synchronized(entries) {
            if (entries.size >= CAPACITY) {
                entries.removeFirst()
            }
            entries.addLast(entry)
        }
        _updates.tryEmit(entry)
    }

    fun snapshot(): List<Entry> = synchronized(entries) { entries.toList() }

    fun clear() = synchronized(entries) { entries.clear() }

    companion object {
        const val CAPACITY = 2000
    }
}
