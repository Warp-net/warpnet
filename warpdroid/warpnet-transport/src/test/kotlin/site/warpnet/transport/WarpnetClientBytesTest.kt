/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class WarpnetClientBytesTest {

    private val config = WarpnetConfig(
        privKeyHex = "ab",
        pskHex = "cd",
        bootstrapAddrs = listOf("/ip4/1.2.3.4/tcp/1/p2p/B"),
        desktopPeerAddr = "",
        network = "testnet",
    )

    private suspend fun connectedClient(binding: BytesBinding): WarpnetClient {
        val client = WarpnetClient(Moshi.Builder().build(), BindingSigner(binding), binding)
        client.initialise(config)
        client.connect(listOf("/a"))
        return client
    }

    @Test
    fun `bytes requests send a signed envelope and retry a stream that never opened`() = runBlocking {
        val video = ByteArray(128 * 1024) { it.toByte() }
        val binding = BytesBinding({ "failed to open stream: dial backoff".toByteArray() }, { video })
        val client = connectedClient(binding)

        assertArrayEquals(video, client.requestBytes("/public/get/video/0.0.0", """{"key":"k"}"""))

        assertEquals(2, binding.calls.size)
        assertTrue(binding.calls.last().startsWith("""{"body":{"key":"k"},"""))
        assertTrue(binding.calls.last().contains(""""signature":"c2ln""""))
    }

    @Test
    fun `a small response is checked for a node error`() = runBlocking {
        val small = """{"file":"","size":0}""".toByteArray()
        assertArrayEquals(small, connectedClient(BytesBinding({ small })).requestBytes("/p", "{}"))

        val nodeError = """{"code":5000,"message":"boom"}""".toByteArray()
        val failure = runCatching {
            connectedClient(BytesBinding({ nodeError })).requestBytes("/p", "{}")
        }.exceptionOrNull()
        assertTrue(failure is WarpnetException.ProtocolError)
    }

    @Test
    fun `a response that may have reached the handler is not retried`() = runBlocking {
        val binding = BytesBinding({ "stream: reading response from P: stream reset".toByteArray() })
        val failure = runCatching { connectedClient(binding).requestBytes("/p", "{}") }.exceptionOrNull()
        assertTrue(failure is WarpnetException.TransportFailure)
        assertEquals(1, binding.calls.size)
    }
}

private class BytesBinding(vararg results: () -> ByteArray) : WarpnetBinding {
    private val results = ConcurrentLinkedQueue(results.toList())
    val calls = CopyOnWriteArrayList<String>()

    override fun streamBytes(protocolId: String, data: String): ByteArray {
        calls.add(data)
        return results.poll()?.invoke() ?: ByteArray(0)
    }

    override fun initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String) = ""
    override fun connect(addrInfo: String) = ""
    override fun stream(protocolId: String, data: String) = "{}"
    override fun peerId() = "12D3KooWTestPeer"
    override fun isConnected() = true
    override fun disconnect() = ""
    override fun pause() {}
    override fun resume() {}
    override fun shutdown() = ""
    override fun sign(body: String) = "c2ln"
    override fun refreshPeerAddrs(addrs: String) = ""
}
