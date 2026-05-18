/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.JsonWriter
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapter
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okio.Buffer
import java.time.Instant
import java.time.format.DateTimeFormatter

/**
 * Coroutine-friendly wrapper around the `node.Node` gomobile binding.
 *
 * Lifecycle:
 * 1. [initialise] — start the libp2p host with the Ed25519 identity, PSK and
 *    bootstrap peers. Idempotent.
 * 2. [connect] — open a connection to the paired desktop multiaddr. Cheap;
 *    the binding keeps a single connection and [request] rides on it.
 * 3. [request] — open one stream per call (`open → write → closeWrite →
 *    read-to-EOF`), as documented in [ProtocolIds] and
 *    `docs/warpnet-protocol.md`.
 * 4. [pause] / [resume] — drive libp2p's connection state across Android
 *    foreground/background transitions without tearing the host down.
 * 5. [shutdown] — close the host. After this the instance must be discarded;
 *    the gomobile side uses a package-level `clientInstance` singleton and
 *    will refuse re-initialisation until `Shutdown` clears it.
 *
 * The binding is globally singleton on the Go side (mobile.go), so this
 * class also serialises access through a mutex. Do not construct more than
 * one [WarpnetClient] per process.
 */
@OptIn(ExperimentalStdlibApi::class)
class WarpnetClient(
    private val moshi: Moshi,
    private val signer: EnvelopeSigner,
    private val binding: WarpnetBinding = DefaultBinding,
) {
    private val mutex = Mutex()
    private val _state = MutableStateFlow<ConnectionState>(ConnectionState.Uninitialised)
    val state: StateFlow<ConnectionState> = _state.asStateFlow()

    private val errorAdapter = moshi.adapter<WarpnetResponseError>()
    private val addrsAdapter = moshi.adapter<List<String>>()

    /** Start the libp2p host. No-op if already initialised. */
    suspend fun initialise(config: WarpnetConfig) = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (_state.value != ConnectionState.Uninitialised) return@withLock
            // Preflight checks throw without mutating state so the caller can
            // retry initialise() with corrected inputs.
            if (config.bootstrapAddrs.isEmpty()) {
                throw WarpnetException.TransportFailure("bootstrap nodes required")
            }
            val err = binding.initialize(
                privKeyHex = config.privKeyHex,
                warpNetwork = config.network,
                pskHex = config.pskHex,
                bootstrapNodes = config.bootstrapAddrs.joinToString(","),
            )
            if (err.isNotEmpty()) {
                val failure = WarpnetException.TransportFailure(err)
                _state.value = ConnectionState.Failed(failure)
                throw failure
            }
            _state.value = ConnectionState.Disconnected
        }
    }

    /**
     * Hand the full set of [candidateAddrs] to the Go binding in a single
     * call so libp2p's swarm.DefaultDialRanker can rank them and dial in
     * parallel. Which address actually wins isn't surfaced — only the
     * binding's connectedness() knows after the fact — so this returns
     * nothing.
     */
    suspend fun connect(candidateAddrs: List<String>) = withContext(Dispatchers.IO) {
        if (candidateAddrs.isEmpty()) {
            throw WarpnetException.TransportFailure("no addresses to dial")
        }
        mutex.withLock {
            if (_state.value == ConnectionState.Uninitialised) {
                throw WarpnetException.NotInitialised()
            }
            _state.value = ConnectionState.Connecting
            val err = binding.connect(candidateAddrs.joinToString("\n"))
            if (err.isNotEmpty()) {
                val failure = WarpnetException.TransportFailure(
                    "$err. On LAN this is most often the PC firewall " +
                        "blocking the libp2p port."
                )
                _state.value = ConnectionState.Failed(failure)
                throw failure
            }
            _state.value = ConnectionState.Connected
        }
    }

    /**
     * Open the pairing stream and submit [rawAuthNodeInfoJson] wrapped in the
     * standard [WarpnetEnvelope]. Every external request — pairing included
     * — must be a `Message` envelope so the server's auth middleware
     * (`warpnet/middleware`) can unmarshal it before dispatching to the pair
     * handler. The body inside the envelope is the AuthNodeInfo JSON the
     * server originally produced and the QR carried, so the pair handler's
     * token comparison still matches byte-for-byte.
     *
     * On success the server returns a JSON array of its current public
     * multiaddrs; merge them into the peerstore so subsequent reconnects
     * have fresh addresses. A [WarpnetResponseError]-shaped envelope is
     * surfaced as [WarpnetException.ProtocolError]; anything else as
     * [WarpnetException.TransportFailure].
     */
    suspend fun pair(rawAuthNodeInfoJson: String) = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (_state.value !is ConnectionState.Connected) {
                throw WarpnetException.NotConnected()
            }

            val envelope = WarpnetEnvelope.unsigned(
                body = rawAuthNodeInfoJson,
                nodeId = signer.peerId,
                path = ProtocolIds.PRIVATE_POST_PAIR,
                timestamp = DateTimeFormatter.ISO_INSTANT.format(Instant.now()),
            ).copy(signature = signer.sign(rawAuthNodeInfoJson))

            val requestJson = buildEnvelopeJson(envelope)
            val raw = binding.stream(ProtocolIds.PRIVATE_POST_PAIR, requestJson)
            val trimmed = raw.trim()
            if (trimmed.isEmpty()) {
                throw WarpnetException.TransportFailure("empty pairing response")
            }
            // ResponseError envelopes start with `{`; the success path is a
            // JSON array of multiaddrs starting with `[`.
            if (trimmed.startsWith("{")) {
                val parsed = runCatching { errorAdapter.fromJson(trimmed) }.getOrNull()
                if (parsed != null && (parsed.code != 0 || parsed.message.isNotEmpty())) {
                    throw WarpnetException.ProtocolError(parsed.code, parsed.message)
                }
                throw WarpnetException.TransportFailure(trimmed)
            }
            val addrs = runCatching { addrsAdapter.fromJson(trimmed) }.getOrNull()
                ?: throw WarpnetException.TransportFailure(
                    "unexpected pairing response: $trimmed"
                )
            if (addrs.isNotEmpty()) {
                val err = binding.refreshPeerAddrs(addrs.joinToString("\n"))
                if (err.isNotEmpty()) {
                    throw WarpnetException.TransportFailure(err)
                }
            }
        }
    }

    /**
     * Send one request over a fresh libp2p stream. [bodyJson] is the raw JSON
     * of the event payload (e.g. a serialised `GetUserEvent`). The envelope
     * wrapping is done here.
     */
    /**
     * Send one request over a fresh libp2p stream and retry transient
     * transport failures with exponential backoff. Failure modes that
     * mean "the request never reached the server" — NewStream timeouts,
     * "not connected to desktop node", context-deadline errors — are
     * retried up to [STREAM_RETRY_BACKOFFS] entries; the connection
     * monitor running in parallel typically re-establishes the link
     * within one or two of those waits. Failure modes that imply the
     * server may have processed the request (response-read mid-flight)
     * are NOT retried to avoid double-applying non-idempotent
     * mutations — see [isRetryableTransportError].
     *
     * Protocol-level errors (4xx-style ResponseError) propagate
     * immediately; retrying them would only annoy the user.
     */
    suspend fun request(protocolId: String, bodyJson: String): String = withContext(Dispatchers.IO) {
        // No mutex: the Go binding's libp2p host is thread-safe and yamux
        // multiplexes concurrent streams over the single connection.
        // Serialising here made every refresh block behind the slowest
        // in-flight call (timeline stalls behind a 15s stream-open).
        var lastFailure: WarpnetException.TransportFailure? = null
        for (attempt in 0..STREAM_RETRY_BACKOFFS.size) {
            if (_state.value !is ConnectionState.Connected) {
                throw WarpnetException.NotConnected()
            }
            // Rebuild + re-sign the envelope each attempt so the
            // timestamp matches the wire and the signature stays valid
            // against the freshly-sent body.
            val envelope = WarpnetEnvelope.unsigned(
                body = bodyJson,
                nodeId = signer.peerId,
                path = protocolId,
                timestamp = DateTimeFormatter.ISO_INSTANT.format(Instant.now()),
            ).copy(signature = signer.sign(bodyJson))

            val requestJson = buildEnvelopeJson(envelope)
            val raw = binding.stream(protocolId, requestJson)

            try {
                throwIfErrorResponse(raw)
                return@withContext raw
            } catch (e: WarpnetException.TransportFailure) {
                if (!isRetryableTransportError(e.message.orEmpty()) ||
                    attempt == STREAM_RETRY_BACKOFFS.size
                ) {
                    throw e
                }
                lastFailure = e
                delay(STREAM_RETRY_BACKOFFS[attempt])
            }
        }
        throw lastFailure ?: WarpnetException.TransportFailure("retry budget exhausted")
    }

    private fun isRetryableTransportError(msg: String): Boolean {
        // Restrict retries to "request never reached the server"
        // categories. Anything that mentions response-side I/O implies
        // the handler may have run, which we mustn't double-apply.
        if (msg.contains("stream: reading response", ignoreCase = true)) return false
        return msg.contains("failed to open stream", ignoreCase = true) ||
            msg.contains("not connected to desktop node", ignoreCase = true) ||
            msg.contains("context deadline exceeded", ignoreCase = true) ||
            msg.contains("stream reset", ignoreCase = true) ||
            msg.contains("muxer closed", ignoreCase = true)
    }

    /**
     * Drop live connections when the app moves to the background. The host
     * remains initialised; [resume] re-dials known peers. Mirrors
     * `node.Node.pause()`.
     */
    suspend fun pause() = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (_state.value == ConnectionState.Uninitialised) return@withLock
            binding.pause()
            // Don't overwrite a Failed state; only demote from Connected/Connecting.
            if (_state.value is ConnectionState.Connected || _state.value is ConnectionState.Connecting) {
                _state.value = ConnectionState.Disconnected
            }
        }
    }

    /**
     * Re-establish connections to previously known peers when the app returns
     * to the foreground. Mirrors `node.Node.resume()`.
     */
    suspend fun resume() = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (_state.value == ConnectionState.Uninitialised) return@withLock
            binding.resume()
            // Resume re-dials known peers in the background. Only promote back
            // to Connected if the native host actually has a live connection;
            // preserve Failed so the UI can still surface the prior error.
            if (_state.value is ConnectionState.Failed) return@withLock
            _state.value = if (binding.isConnected()) {
                ConnectionState.Connected
            } else {
                ConnectionState.Disconnected
            }
        }
    }

    /** Close the native host. After this the client must be discarded. */
    suspend fun shutdown() = withContext(Dispatchers.IO) {
        mutex.withLock {
            binding.shutdown()
            _state.value = ConnectionState.Uninitialised
        }
    }

    /**
     * Marshal [envelope] to JSON with `body` emitted as raw JSON instead of a
     * quoted string. The server's `event.Message.Body` is `json.RawMessage`,
     * so the handler unmarshals those bytes directly as the inner event
     * type. Moshi's default `String` adapter would JSON-escape the body,
     * yielding `"body":"{\"foo\":1}"` and breaking server-side unmarshal:
     *
     *     pair: unmarshaling from stream: "{\"user_id\":...}"
     *     readObjectStart: expect { or n, but found "
     *
     * `JsonWriter.valueSink()` writes the next value's bytes verbatim,
     * preserving byte-for-byte parity with what was signed (so the
     * signature still verifies on the server side).
     */
    private fun buildEnvelopeJson(envelope: WarpnetEnvelope): String {
        val buf = Buffer()
        JsonWriter.of(buf).use { w ->
            w.beginObject()
            w.name("body")
            w.valueSink().use { it.writeUtf8(envelope.body) }
            w.name("message_id").value(envelope.message_id)
            w.name("node_id").value(envelope.node_id)
            w.name("path").value(envelope.path)
            w.name("timestamp").value(envelope.timestamp)
            w.name("version").value(envelope.version)
            w.name("signature").value(envelope.signature)
            w.endObject()
        }
        return buf.readUtf8()
    }

    private fun throwIfErrorResponse(raw: String) {
        if (raw.isEmpty()) return
        // A well-formed ResponseError is exactly `{"code":N,"message":"..."}`.
        // Anything else is either a valid payload or a transport-layer error
        // string from the binding. Heuristic: only try to parse if the first
        // non-whitespace char is `{` and the JSON has "code" + "message".
        if (!raw.trimStart().startsWith("{")) {
            throw WarpnetException.TransportFailure(raw)
        }
        val err = runCatching { errorAdapter.fromJson(raw) }.getOrNull() ?: return
        if (err.code == 0 && err.message.isEmpty()) return
        throw WarpnetException.ProtocolError(err.code, err.message)
    }

    private companion object {
        // Backoff between retries on transient transport failures. Total
        // worst-case wait is ~9s plus the underlying stream timeouts, so
        // a paged getTimeline still returns within ~30s on a flaky link
        // before the ViewModel decides to show "an error occurred".
        val STREAM_RETRY_BACKOFFS = longArrayOf(500L, 1_500L, 4_000L, 8_000L)
    }
}

/**
 * Indirection over the gomobile binding so unit tests can swap it out. The
 * real implementation forwards straight to `node.Node`.
 */
interface WarpnetBinding {
    fun initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String): String
    fun connect(addrInfo: String): String
    fun stream(protocolId: String, data: String): String
    fun peerId(): String
    fun isConnected(): Boolean
    /**
     * Current libp2p Connectedness for the paired desktop peer. Polled
     * by [ConnectionMonitor]; see [LinkState.fromBinding] for the
     * mapping of returned strings to UI-level state.
     *
     * The default falls back to a binary "Connected" / "NotConnected"
     * read from [isConnected] so the interface can be added without
     * requiring an AAR rebuild — once the freshly generated gomobile
     * binding exposes node.Node.connectedness(), [DefaultBinding]
     * should override this to return the proper three-state value
     * including "Limited" (relay-only).
     */
    fun connectedness(): String = if (isConnected()) "Connected" else "NotConnected"
    fun disconnect(): String
    fun pause()
    fun resume()
    fun shutdown(): String

    /**
     * Sign [body] with the libp2p identity key passed to [initialize] and
     * return the base64-encoded Ed25519 signature. Returns an empty string
     * if the binding is not initialised, or an "error: ..." string on
     * signing failure (see warpdroid/node/mobile.go:Sign).
     */
    fun sign(body: String): String

    /**
     * Merge newline-separated [addrs] into the libp2p peerstore for the
     * paired desktop peer. Empty string on success, error message otherwise.
     * The pair handler returns the fat node's current public addresses on
     * every successful call; periodically re-pairing keeps the peerstore
     * fresh when the fat node moves between networks.
     */
    fun refreshPeerAddrs(addrs: String): String
}

/**
 * Thin forwarder to the gomobile-generated `node.Node` class. Kept as an
 * `object` so we don't carry any state on the Kotlin side that might desync
 * from the Go singleton.
 *
 * Method names are camelCase because gomobile lowercases the first letter of
 * Go-exported functions when generating Java bindings.
 */
object DefaultBinding : WarpnetBinding {
    override fun initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String): String =
        node.Node.initialize(privKeyHex, warpNetwork, pskHex, bootstrapNodes)

    override fun connect(addrInfo: String): String =
        node.Node.connect(addrInfo)

    override fun stream(protocolId: String, data: String): String =
        node.Node.stream(protocolId, data)

    override fun peerId(): String = node.Node.peerID()

    override fun isConnected(): Boolean = node.Node.isConnected() == "true"

    // Once the AAR is rebuilt against mobile.go's Connectedness() export
    // (`make gen-aar`), override this to call node.Node.connectedness()
    // directly for the three-state Connected / Limited / NotConnected
    // distinction. Until then the interface default keeps CI green by
    // collapsing to the two-state isConnected() answer.

    override fun disconnect(): String = node.Node.disconnect()

    override fun pause() = node.Node.pause()

    override fun resume() = node.Node.resume()

    override fun shutdown(): String = node.Node.shutdown()

    override fun sign(body: String): String = node.Node.sign(body)

    override fun refreshPeerAddrs(addrs: String): String =
        node.Node.refreshPeerAddrs(addrs)
}
