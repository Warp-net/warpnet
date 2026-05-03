/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

/**
 * Abstraction over the Ed25519 signature that Warpnet's middleware verifies
 * against the libp2p peer ID on every inbound request (see
 * warpnet/core/middleware/middleware.go:91).
 *
 * The default production implementation is [BindingSigner], which delegates
 * signing to the gomobile binding so the same libp2p identity key signs every
 * request. [NoOpSigner] remains for unit tests that exercise envelope shape
 * without real crypto.
 */
interface EnvelopeSigner {
    /**
     * Sign the raw body bytes and return a base64-encoded Ed25519 signature,
     * or throw [WarpnetException.SigningUnavailable] if the current binding
     * cannot produce a verifying signature.
     */
    fun sign(bodyJson: String): String

    /** The peer ID of the signing key, matching libp2p's FromIDToPubKey. */
    val peerId: String
}

/**
 * Always throws. Use only in unit tests that need an [EnvelopeSigner]
 * instance but never invoke [sign]; production wiring uses [BindingSigner].
 */
class NoOpSigner(override val peerId: String = "") : EnvelopeSigner {
    override fun sign(bodyJson: String): String {
        throw WarpnetException.SigningUnavailable(
            "NoOpSigner cannot produce a signature; wire BindingSigner instead."
        )
    }
}

/**
 * Production [EnvelopeSigner] that defers to the gomobile binding. The native
 * node holds the Ed25519 identity (passed in via Initialize) and signs the
 * body with the same key libp2p uses for the connection peer ID, so the
 * server's auth middleware verifies the signature against the peer it sees
 * on the stream.
 *
 * Both [peerId] and [sign] read through the binding lazily so the signer can
 * be constructed before [WarpnetClient.initialise]; calls before init will
 * see an empty peer ID / blank signature and surface as
 * [WarpnetException.SigningUnavailable].
 */
class BindingSigner(private val binding: WarpnetBinding) : EnvelopeSigner {
    override val peerId: String get() = binding.peerId()

    override fun sign(bodyJson: String): String {
        val sig = binding.sign(bodyJson)
        if (sig.isEmpty() || sig.startsWith("error:")) {
            throw WarpnetException.SigningUnavailable(
                if (sig.isEmpty()) "binding not initialised" else sig
            )
        }
        return sig
    }
}
