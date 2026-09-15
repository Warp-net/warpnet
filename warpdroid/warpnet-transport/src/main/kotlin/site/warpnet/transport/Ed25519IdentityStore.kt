/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.google.crypto.tink.subtle.Ed25519Sign

/**
 * Keeps the 32-byte seed the device's libp2p identity is built from. The
 * seed is the credential a paired node authorizes, so it must come from a
 * CSPRNG and be persisted; it is implemented on the app side, next to the
 * pairing payload it shares a fate with.
 */
interface IdentitySeedStore {
    /**
     * The seed for the pairing with [memberPeerId], created on first use.
     * Returns the same 32 bytes for every later call until the pairing is
     * cleared.
     */
    fun seed(memberPeerId: String): ByteArray
}

/**
 * Builds the 64-byte libp2p Ed25519 private key the AAR's `Initialize` method
 * consumes from the seed held by [IdentitySeedStore]. The output format is
 * `seed(32) || publicKey(32)` as expected by
 * `crypto.UnmarshalEd25519PrivateKey` in go-libp2p.
 *
 * The identity lives as long as the pairing does: it is created when a node
 * is paired with, survives restarts, and is gone once the pairing is cleared
 * — unpairing therefore retires the identity rather than leaving a key the
 * device could present again.
 */
class Ed25519IdentityStore(private val seeds: IdentitySeedStore) {

    /**
     * Derive the libp2p identity for the given member (fat) node peer ID.
     * Returns 64 raw bytes (seed || public key). Touches storage, so call
     * it off the main thread.
     */
    fun derive(memberPeerId: String): ByteArray {
        require(memberPeerId.isNotEmpty()) { "memberPeerId must not be empty" }
        val seed = seeds.seed(memberPeerId)
        require(seed.size == SEED_SIZE) { "Unexpected Ed25519 seed length (${seed.size})" }
        // Tink's standalone Ed25519 implementation expands a 32-byte seed
        // into a (publicKey, secretSeed) pair without needing a JCE
        // provider, which the platform AndroidOpenSSL provider does not
        // register on min SDK 24.
        val kp = Ed25519Sign.KeyPair.newKeyPairFromSeed(seed)
        val pub = kp.publicKey
        require(pub.size == PUB_SIZE) { "Unexpected Ed25519 public key length (${pub.size})" }
        return seed + pub
    }

    /** Same as [derive] but hex-encoded (lowercase), ready for the AAR. */
    @OptIn(ExperimentalStdlibApi::class)
    fun deriveHex(memberPeerId: String): String = derive(memberPeerId).toHexString()

    private companion object {
        const val SEED_SIZE = 32
        const val PUB_SIZE = 32
    }
}
