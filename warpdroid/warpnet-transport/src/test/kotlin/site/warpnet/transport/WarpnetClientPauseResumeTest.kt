/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class WarpnetClientPauseResumeTest {

    private val config = WarpnetConfig(
        privKeyHex = "ab",
        pskHex = "cd",
        bootstrapAddrs = listOf("/ip4/1.2.3.4/tcp/1/p2p/B"),
        desktopPeerAddr = "",
        network = "testnet",
    )

    private fun makeClient(binding: FakeBinding) =
        WarpnetClient(Moshi.Builder().build(), BindingSigner(binding), binding)

    private suspend fun connectedClient(binding: FakeBinding): WarpnetClient {
        val client = makeClient(binding)
        client.initialise(config)
        client.connect(listOf("/a"))
        return client
    }

    @Test
    fun `a paused client fails fast and resume does not wait for the redial`() = runBlocking {
        val binding = FakeBinding()
        val client = connectedClient(binding)
        client.pause()
        assertEquals(ConnectionState.Disconnected, client.state.value)
        assertEquals(1, binding.pauseCount.get())
        val failure = runCatching { client.request("/p", "{}") }.exceptionOrNull()
        assertTrue("a paused client must not dial", failure is WarpnetException.NotConnected)

        // The fake reports no connection, like the node while its redial is in flight.
        client.resume()
        assertEquals(ConnectionState.Connected, client.state.value)
        assertEquals(1, binding.resumeCount.get())
        client.request("/p", "{}")
        assertEquals(1, binding.streamCalls.size)
    }

    @Test
    fun `resume only restores a connection the pause took down`() = runBlocking {
        val client = connectedClient(FakeBinding())
        client.pause()
        client.shutdown()
        client.initialise(config)
        client.resume()
        assertEquals(ConnectionState.Disconnected, client.state.value)
    }

    @Test
    fun `a request waits for a reconnect in flight`() = runBlocking {
        val binding = FakeBinding()
        val client = connectedClient(binding)
        val gate = CountDownLatch(1)
        binding.connectGate = gate
        val reconnect = async(Dispatchers.IO) { client.connect(listOf("/a")) }
        client.state.first { it == ConnectionState.Connecting }
        val request = async(Dispatchers.IO) { client.request("/p", "{}") }
        delay(50)
        assertTrue(binding.streamCalls.isEmpty())
        gate.countDown()
        reconnect.await()
        request.await()
        assertEquals(1, binding.streamCalls.size)
    }

    @Test
    fun `resume keeps a failure`() = runBlocking {
        val binding = FakeBinding()
        binding.connectResult = "connection failed"
        val client = makeClient(binding)
        client.initialise(config)
        runCatching { client.connect(listOf("/a")) }
        client.resume()
        assertTrue(client.state.value is ConnectionState.Failed)
    }
}

private class FakeBinding : WarpnetBinding {
    @Volatile var connectResult = ""
    @Volatile var connectGate: CountDownLatch? = null
    val streamCalls = CopyOnWriteArrayList<String>()
    val pauseCount = AtomicInteger()
    val resumeCount = AtomicInteger()

    override fun initialize(
        privKeyHex: String,
        warpNetwork: String,
        pskHex: String,
        bootstrapNodes: String,
    ): String = ""

    override fun connect(addrInfo: String): String {
        // Bounded so a regression fails the test instead of hanging it.
        connectGate?.await(10, TimeUnit.SECONDS)
        return connectResult
    }

    override fun stream(protocolId: String, data: String): String {
        streamCalls.add(protocolId)
        return "{}"
    }

    override fun peerId(): String = "12D3KooWTestPeer"
    override fun isConnected(): Boolean = false
    override fun disconnect(): String = ""
    override fun pause() {
        pauseCount.incrementAndGet()
    }

    override fun resume() {
        resumeCount.incrementAndGet()
    }

    override fun shutdown(): String = ""
    override fun sign(body: String): String = "c2ln"
    override fun refreshPeerAddrs(addrs: String): String = ""
}
