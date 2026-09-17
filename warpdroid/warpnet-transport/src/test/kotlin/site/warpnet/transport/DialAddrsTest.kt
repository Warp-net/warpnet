/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DialAddrsTest {
    private val peerId = "12D3KooWRHVAeNyW1CsGT4Rm2adMY7oEkRZ5EpvGgAXvFVzX1BAc"

    @Test
    fun `keeps the local network address and drops what a device cannot reach`() {
        val announced = listOf(
            "/ip4/127.0.0.1/tcp/4001",
            "/ip4/192.168.1.138/tcp/4001",
            "/ip6/::1/tcp/4001",
            "/ip4/169.254.3.7/tcp/4001",
            "/ip6/fe80::1/tcp/4001",
            "/ip4/0.0.0.0/tcp/4001",
            "/ip4/95.164.7.11/tcp/4001",
        )

        assertEquals(
            listOf(
                "/ip4/192.168.1.138/tcp/4001/p2p/$peerId",
                "/ip4/95.164.7.11/tcp/4001/p2p/$peerId",
            ),
            dialAddrs(announced, peerId),
        )
    }

    @Test
    fun `keeps every private range`() {
        val announced = listOf(
            "/ip4/10.1.2.3/tcp/4001",
            "/ip4/172.17.0.1/tcp/4001",
            "/ip4/192.168.1.138/tcp/4001",
        )

        assertEquals(announced.size, dialAddrs(announced, peerId).size)
    }

    @Test
    fun `an address that is only loopback yields nothing to dial`() {
        assertTrue(dialAddrs(listOf("/ip4/127.0.0.1/tcp/4001"), peerId).isEmpty())
    }

    @Test
    fun `matches loopback on the whole address component`() {
        assertEquals(
            listOf("/ip6/::123/tcp/4001/p2p/$peerId"),
            dialAddrs(listOf("/ip6/::123/tcp/4001"), peerId),
        )
    }
}
