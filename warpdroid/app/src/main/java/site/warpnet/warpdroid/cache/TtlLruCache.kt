/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.cache

/**
 * General-purpose, thread-safe in-memory cache with per-entry TTL and an
 * LRU size bound. Warpdroid is a thin client to the user's fat node, so any
 * data that's expensive to re-fetch over the relay (profiles, tweet stats,
 * relationships, …) can be parked here instead of paying a round-trip on
 * every screen. Each data type creates its own typed instance with the TTL
 * and capacity that suit it.
 *
 * Semantics:
 *  - [get] returns null once an entry is older than [ttlMillis], removing it
 *    on the way out so stale objects don't linger.
 *  - Access order drives eviction: once the map exceeds [maxSize] the
 *    least-recently-used entry is dropped, bounding memory on low-end
 *    devices.
 *
 * All public operations synchronize on the instance, so concurrent
 * coroutines (e.g. a timeline hydration fan-out) can share one cache without
 * external coordination. [nowMillis] is injectable for deterministic tests.
 */
class TtlLruCache<K : Any, V : Any>(
    private val maxSize: Int,
    private val ttlMillis: Long,
    private val nowMillis: () -> Long = System::currentTimeMillis,
) {
    private data class Entry<V>(val value: V, val storedAt: Long)

    private val map = object : LinkedHashMap<K, Entry<V>>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<K, Entry<V>>): Boolean =
            size > maxSize
    }

    @Synchronized
    fun get(key: K): V? {
        val entry = map[key] ?: return null
        if (nowMillis() - entry.storedAt >= ttlMillis) {
            map.remove(key)
            return null
        }
        return entry.value
    }

    @Synchronized
    fun put(key: K, value: V) {
        map[key] = Entry(value, nowMillis())
    }

    /** Return the cached value if fresh, otherwise compute, store and return it. */
    suspend fun getOrLoad(key: K, loader: suspend () -> V?): V? {
        get(key)?.let { return it }
        return loader()?.also { put(key, it) }
    }

    @Synchronized
    fun invalidate(key: K) {
        map.remove(key)
    }

    @Synchronized
    fun clear() {
        map.clear()
    }

    @get:Synchronized
    val size: Int get() = map.size
}
