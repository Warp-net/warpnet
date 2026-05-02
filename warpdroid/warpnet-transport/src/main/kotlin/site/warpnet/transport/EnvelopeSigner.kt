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
 * Why this is an interface instead of a concrete impl:
 *
 * The current `warp-net/android-binding` generates its Ed25519 keypair
 * internally and does not expose it (android-binding/client.go:41). That
 * means Kotlin cannot produce a signature that matches the peer ID the node
 * observes on the stream. Until the binding grows either a `Sign(body)`
 * export or a `SetIdentityKey(pem)` setter, the only working signer is
 * [NoOpSigner] — which lets the envelope flow through for protocol IDs that
 * the node's middleware does not verify (there aren't many in practice),
 * and lets unit tests exercise the envelope shape without real crypto.
 *
 * Once the binding is updated, implement [BindingSigner] that delegates to
 * the new export, register it via Hilt, and retire [NoOpSigner].
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
 * Returns an empty signature and an empty peer ID. Use this for development
 * against Warpnet endpoints that are not signature-verified (the minority),
 * or for unit tests. Do not ship to users.
 */
class NoOpSigner(override val peerId: String = "") : EnvelopeSigner {
    override fun sign(bodyJson: String): String {
        throw WarpnetException.SigningUnavailable(
            "Envelope signing requires warp-net/android-binding to export the " +
                "node's Ed25519 private key (or sign internally). See " +
                "docs/warpnet-protocol.md §'Client-side implications'."
        )
    }
}
