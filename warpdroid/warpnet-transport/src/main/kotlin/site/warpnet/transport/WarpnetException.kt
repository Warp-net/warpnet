/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * Every error surface the transport layer can hand to callers. Callers should
 * be able to map these to user-facing copy without inspecting the binding's
 * stringly-typed error returns directly.
 */
sealed class WarpnetException(message: String, cause: Throwable? = null) : Exception(message, cause) {

    /** The native node has not been initialised yet. */
    class NotInitialised : WarpnetException("Warpnet transport is not initialised")

    /** No desktop peer has been paired / connected. */
    class NotConnected : WarpnetException("Not connected to a Warpnet desktop node")

    /** Stream opened but the desktop rejected the request. Message is from the node. */
    class ProtocolError(val code: Int, val serverMessage: String) :
        WarpnetException("Warpnet error $code: $serverMessage")

    /** libp2p-level transport failure (TCP, Noise, handshake). */
    class TransportFailure(message: String, cause: Throwable? = null) :
        WarpnetException(message, cause)

    /** JSON could not be parsed / produced. */
    class SerializationError(message: String, cause: Throwable? = null) :
        WarpnetException(message, cause)

    /**
     * The transport could not produce a signature for the outgoing envelope,
     * usually because the binding is not initialised yet or returned an
     * error from its native signing path. The desktop-side auth middleware
     * rejects unsigned envelopes, so callers should treat this as a fatal
     * setup error rather than retry.
     */
    class SigningUnavailable(message: String) : WarpnetException(message)
}
