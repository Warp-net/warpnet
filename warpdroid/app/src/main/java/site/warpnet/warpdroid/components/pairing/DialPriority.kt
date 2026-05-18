/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Defensive client-side classification of dial candidates. The fat node
 * already sorts the addresses it puts into the pairing QR (core/node/
 * node.go::BaseNodeInfo applies the same tiering server-side), but a
 * phone may be paired against an older desktop build that didn't, so
 * the thin client re-applies the filter+sort here and never trusts the
 * QR's raw order. WarpnetClient.connectAny iterates in list order and
 * stops at the first successful dial, so getting this wrong means the
 * pairing routes through a public circuit-v2 relay on the same Wi-Fi
 * where a LAN dial would have been sub-millisecond.
 */
package site.warpnet.warpdroid.components.pairing

/**
 * Drop multiaddrs that can never be dialed from the phone (loopback,
 * link-local, docker bridges, IPv6 loopback) and stable-sort the rest
 * so direct dials win before relayed ones:
 *
 *  1. RFC1918 / IPv6 ULA — preferred when phone is on the same network.
 *  2. Public-direct IPv4/IPv6 — global address without `p2p-circuit`.
 *  3. `…/p2p-circuit` — transit-bound, always reachable, slowest.
 *
 * Stability within a tier preserves the order the fat node advertised,
 * which matters when a host has multiple interfaces on the same tier.
 */
fun prioritizeDialAddresses(raw: List<String>): List<String> {
    val ranked = raw.mapIndexedNotNull { idx, addr ->
        val tier = classify(addr)
        if (tier == TIER_DROP) null else Triple(addr, tier, idx)
    }
    return ranked
        .sortedWith(compareBy({ it.second }, { it.third }))
        .map { it.first }
}

private const val TIER_LAN = 0
private const val TIER_PUBLIC = 1
private const val TIER_RELAY = 2
private const val TIER_DROP = Int.MAX_VALUE

private fun classify(maddr: String): Int {
    if (maddr.contains("/p2p-circuit")) return TIER_RELAY
    extractIPv4(maddr)?.let { return classifyIPv4(it) }
    extractIPv6(maddr)?.let { return classifyIPv6(it) }
    // dnsaddr / dns4 / dns6 — assume routable, defer to dial layer.
    if (maddr.contains("/dns")) return TIER_PUBLIC
    return TIER_DROP
}

private fun classifyIPv4(ip: String): Int {
    val parts = ip.split('.').mapNotNull { it.toIntOrNull() }
    if (parts.size != 4 || parts.any { it !in 0..255 }) return TIER_DROP
    val (a, b) = parts[0] to parts[1]
    return when {
        a == 0 -> TIER_DROP
        a == 127 -> TIER_DROP                    // loopback
        a == 169 && b == 254 -> TIER_DROP        // link-local
        a in 224..239 -> TIER_DROP               // multicast
        a == 172 && b in 17..18 -> TIER_DROP     // docker default bridges
        a == 10 -> TIER_LAN
        a == 172 && b in 16..31 -> TIER_LAN
        a == 192 && b == 168 -> TIER_LAN
        a == 100 && b in 64..127 -> TIER_LAN     // CG-NAT (mobile hotspot)
        else -> TIER_PUBLIC
    }
}

private fun classifyIPv6(ip: String): Int {
    val lower = ip.lowercase()
    return when {
        lower == "::" || lower == "::1" -> TIER_DROP
        lower.startsWith("fe80:") -> TIER_DROP   // link-local
        lower.startsWith("ff") -> TIER_DROP      // multicast
        lower.startsWith("fc") || lower.startsWith("fd") -> TIER_LAN // ULA
        else -> TIER_PUBLIC
    }
}

private fun extractIPv4(maddr: String): String? {
    val marker = "/ip4/"
    val i = maddr.indexOf(marker)
    if (i < 0) return null
    val tail = maddr.substring(i + marker.length)
    val end = tail.indexOf('/')
    return if (end < 0) tail else tail.substring(0, end)
}

private fun extractIPv6(maddr: String): String? {
    val marker = "/ip6/"
    val i = maddr.indexOf(marker)
    if (i < 0) return null
    val tail = maddr.substring(i + marker.length)
    val end = tail.indexOf('/')
    return if (end < 0) tail else tail.substring(0, end)
}
