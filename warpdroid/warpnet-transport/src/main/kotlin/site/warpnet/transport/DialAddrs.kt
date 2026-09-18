/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

fun dialAddrs(addresses: List<String>, peerId: String): List<String> =
    addresses.filterNot(::isUnreachableFromDevice).map { "$it/p2p/$peerId" }

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
