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

    /** Dial the paired desktop peer. */
    suspend fun connect(desktopPeerAddr: String) = withContext(Dispatchers.IO) {
        mutex.withLock {
            if (_state.value == ConnectionState.Uninitialised) {
                throw WarpnetException.NotInitialised()
            }
            _state.value = ConnectionState.Connecting
            val err = binding.connect(desktopPeerAddr)
            if (err.isNotEmpty()) {
                val failure = WarpnetException.TransportFailure(err)
                _state.value = ConnectionState.Failed(failure)
                throw failure
            }
            _state.value = ConnectionState.Connected
        }
    }

    /**
     * Walk [candidateAddrs] in order with a per-address 10s cap; the first
     * address that succeeds becomes the live connection. Underlying Go dial
     * keeps its own 30s ceiling, so this timeout only bounds how long the
     * caller waits before moving on to the next address. On LAN the likely
     * cause of a silent hang is the PC firewall, so surface that hint when
     * every candidate fails.
     */
    suspend fun connectAny(candidateAddrs: List<String>): String = withContext(Dispatchers.IO) {
        if (candidateAddrs.isEmpty()) {
            throw WarpnetException.TransportFailure("no addresses to dial")
        }
        val errors = mutableListOf<String>()
        for (addr in candidateAddrs) {
            val err = runCatching {
                kotlinx.coroutines.withTimeoutOrNull(DIAL_TIMEOUT_MILLIS) {
                    mutex.withLock {
                        if (_state.value == ConnectionState.Uninitialised) {
                            throw WarpnetException.NotInitialised()
                        }
                        _state.value = ConnectionState.Connecting
                        binding.connect(addr)
                    }
                } ?: "timed out after ${DIAL_TIMEOUT_MILLIS}ms"
            }.getOrElse { it.message ?: it.toString() }
            if (err.isEmpty()) {
                _state.value = ConnectionState.Connected
                return@withContext addr
            }
            errors += "$addr: $err"
        }
        val failure = WarpnetException.TransportFailure(
            "could not dial any fat-node address (${errors.joinToString("; ")}). " +
                "On LAN this is most often the PC firewall blocking the libp2p port."
        )
        _state.value = ConnectionState.Failed(failure)
        throw failure
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
     * Expected response is exactly [ACCEPTED_RESPONSE]; anything else is
     * surfaced as a [WarpnetException.ProtocolError] so the caller can show
     * the server message to the user without persisting identity.
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
            if (trimmed == ACCEPTED_RESPONSE) return@withLock
            val parsed = runCatching { errorAdapter.fromJson(trimmed) }.getOrNull()
            if (parsed != null) {
                throw WarpnetException.ProtocolError(parsed.code, parsed.message)
            }
            throw WarpnetException.TransportFailure(
                if (trimmed.isEmpty()) "empty pairing response" else trimmed
            )
        }
    }

    /**
     * Send one request over a fresh libp2p stream. [bodyJson] is the raw JSON
     * of the event payload (e.g. a serialised `GetUserEvent`). The envelope
     * wrapping is done here.
     */
    suspend fun request(protocolId: String, bodyJson: String): String = withContext(Dispatchers.IO) {
        // Serialise stream() with every other binding call so pause/resume
        // can't tear a connection out from under an in-flight request and
        // race the Go singleton's internal state.
        mutex.withLock {
            if (_state.value !is ConnectionState.Connected) {
                throw WarpnetException.NotConnected()
            }

            val envelope = WarpnetEnvelope.unsigned(
                body = bodyJson,
                nodeId = signer.peerId,
                path = protocolId,
                timestamp = DateTimeFormatter.ISO_INSTANT.format(Instant.now()),
            ).copy(signature = signer.sign(bodyJson))

            val requestJson = buildEnvelopeJson(envelope)
            val raw = binding.stream(protocolId, requestJson)

            // The binding returns the error message directly (no JSON wrapping)
            // when the stream itself fails — e.g. peer disconnected. Distinguish
            // by attempting to parse a ResponseError; if that fails assume the
            // string is a ProtocolError body.
            throwIfErrorResponse(raw)
            raw
        }
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
        const val DIAL_TIMEOUT_MILLIS = 10_000L
        // Matches event.Accepted in warpnet/event/event.go; compared verbatim.
        const val ACCEPTED_RESPONSE = "{\"code\":0,\"message\":\"Accepted\"}"
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
    fun disconnect(): String
    fun pause()
    fun resume()
    fun shutdown(): String
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

    override fun disconnect(): String = node.Node.disconnect()

    override fun pause() = node.Node.pause()

    override fun resume() = node.Node.resume()

    override fun shutdown(): String = node.Node.shutdown()
}
