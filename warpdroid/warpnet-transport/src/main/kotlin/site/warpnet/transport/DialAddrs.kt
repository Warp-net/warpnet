/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * [relays] are the node's bootstrap peers, where it holds its reservations.
 * The announced [addresses] are taken before those exist, so circuits are
 * built here: away from home they are the only way to reach the node.
 */
fun dialAddrs(
    addresses: List<String>,
    peerId: String,
    relays: List<String> = emptyList(),
): List<String> {
    val direct = addresses.filterNot(::isUnreachableFromDevice).map { "$it/p2p/$peerId" }
    val relayed = relays.filterNot { it.endsWith("/p2p/$peerId") }
        .map { "$it/p2p-circuit/p2p/$peerId" }
    return (direct + relayed).distinct()
}

private val unreachablePrefixes = listOf(
    "/ip4/127.",
    "/ip4/0.0.0.0/",
    "/ip4/169.254.",
    "/ip6/::1/",
    "/ip6/::/",
    "/ip6/fe80:",
    "/ip6zone/",
)

internal fun isUnreachableFromDevice(addr: String): Boolean =
    unreachablePrefixes.any { addr.startsWith(it, ignoreCase = true) }
