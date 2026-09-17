/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * Turns the addresses a fat node announced into a dial list: each one
 * terminated with `/p2p/<peerId>` so the Noise handshake verifies the
 * remote identity, minus the ones no device can ever reach.
 *
 * A node announces every address it listens on, its own loopback included.
 * Those cost a dial slot each and push the usable LAN address further down
 * libp2p's ranking, which is how a phone sitting on the same Wi-Fi ends up
 * talking to the desktop through a relay.
 */
fun dialAddrs(addresses: List<String>, peerId: String): List<String> =
    addresses.filterNot(::isUnreachableFromDevice).map { "$it/p2p/$peerId" }

// Loopback, unspecified and link-local addresses. Private ranges proper
// (10/8, 172.16/12, 192.168/16) are kept: the node's real LAN address lives
// there, and a container bridge is indistinguishable from it by address
// alone.
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
