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
    fun `reaches the node through every relay after its direct addresses`() {
        val relay = "/ip4/207.154.221.44/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"

        assertEquals(
            listOf(
                "/ip4/192.168.1.138/tcp/4001/p2p/$peerId",
                "$relay/p2p-circuit/p2p/$peerId",
            ),
            dialAddrs(listOf("/ip4/192.168.1.138/tcp/4001"), peerId, listOf(relay)),
        )
    }

    @Test
    fun `a circuit the node already announced is dialled once`() {
        val relay = "/ip4/207.154.221.44/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"

        assertEquals(
            listOf("$relay/p2p-circuit/p2p/$peerId"),
            dialAddrs(listOf("$relay/p2p-circuit"), peerId, listOf(relay)),
        )
    }

    @Test
    fun `the node is never its own relay`() {
        assertTrue(dialAddrs(emptyList(), peerId, listOf("/ip4/1.2.3.4/tcp/4001/p2p/$peerId")).isEmpty())
    }

    @Test
    fun `matches loopback on the whole address component`() {
        assertEquals(
            listOf("/ip6/::123/tcp/4001/p2p/$peerId"),
            dialAddrs(listOf("/ip6/::123/tcp/4001"), peerId),
        )
    }
}
