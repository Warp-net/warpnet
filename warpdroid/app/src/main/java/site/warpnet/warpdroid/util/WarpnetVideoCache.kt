/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.util

import android.util.LruCache
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import site.warpnet.warpdroid.warpnet.WarpnetRepository

private const val MAX_CACHED_VIDEOS = 1

/**
 * The last video played, in memory, so a seek or a replay does not fetch it
 * again; a video is fetched at most once at a time. A key is the hash of the
 * video's content, so the entry never goes stale.
 */
@Singleton
class WarpnetVideoCache @Inject constructor(
    private val repo: WarpnetRepository,
) {
    private val videos = LruCache<String, ByteArray>(MAX_CACHED_VIDEOS)
    private val fetches = ConcurrentHashMap<String, Mutex>()

    internal suspend fun get(ref: WarpnetVideoRef): ByteArray? {
        val name = "${ref.chat}/${ref.key}"
        videos.get(name)?.let { return it }
        return fetches.computeIfAbsent(name) { Mutex() }.withLock {
            videos.get(name) ?: run {
                // Drop the previous video first, so it does not sit next to the new one at the peak.
                videos.evictAll()
                repo.getVideoBytes(ref.userId, ref.key, ref.chat)?.also { videos.put(name, it) }
            }
        }
    }
}
