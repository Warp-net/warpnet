/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import android.os.Build
import com.google.crypto.tink.subtle.Ed25519Sign
import java.security.MessageDigest

/**
 * Builds the 64-byte libp2p Ed25519 private key the AAR's `Initialize` method
 * consumes by deterministically deriving it from the device's
 * [android.os.Build] fingerprint and the paired member node's peer ID. The
 * output format is `seed(32) || publicKey(32)` as expected by
 * `crypto.UnmarshalEd25519PrivateKey` in go-libp2p.
 *
 * Because the key is fully derived, there is nothing to persist: the same
 * (device, member peer ID) pair always produces the same identity, and
 * pairing against a different fat node rotates it. Reinstalling the app does
 * not change the identity for a given fat node, but a factory reset that
 * alters the Build fingerprint will.
 */
class Ed25519IdentityStore {

    /**
     * Derive the libp2p identity for the given member (fat) node peer ID.
     * Returns 64 raw bytes (seed || public key).
     */
    fun derive(memberPeerId: String): ByteArray {
        require(memberPeerId.isNotEmpty()) { "memberPeerId must not be empty" }
        val seed = sha256(deviceMaterial() + "|" + memberPeerId)
        // Tink's standalone Ed25519 implementation expands a 32-byte seed
        // into a (publicKey, secretSeed) pair without needing a JCE
        // provider, which the platform AndroidOpenSSL provider does not
        // register on min SDK 24.
        val kp = Ed25519Sign.KeyPair.newKeyPairFromSeed(seed)
        val pub = kp.publicKey
        require(seed.size == SEED_SIZE && pub.size == PUB_SIZE) {
            "Unexpected Ed25519 key lengths (seed=${seed.size}, pub=${pub.size})"
        }
        return seed + pub
    }

    /** Same as [derive] but hex-encoded (lowercase), ready for the AAR. */
    @OptIn(ExperimentalStdlibApi::class)
    fun deriveHex(memberPeerId: String): String = derive(memberPeerId).toHexString()

    /**
     * Concatenate the [android.os.Build] fields that identify the
     * hardware/firmware combination. FINGERPRINT alone covers most devices,
     * but we mix in the explicit fields so a vendor whose fingerprint comes
     * back empty (seen on some emulators) still yields a non-degenerate
     * seed.
     */
    private fun deviceMaterial(): String = listOf(
        Build.BRAND,
        Build.MANUFACTURER,
        Build.MODEL,
        Build.DEVICE,
        Build.BOARD,
        Build.HARDWARE,
        Build.PRODUCT,
        Build.FINGERPRINT,
        Build.ID,
    ).joinToString("|")

    private fun sha256(input: String): ByteArray =
        MessageDigest.getInstance("SHA-256").digest(input.toByteArray(Charsets.UTF_8))

    private companion object {
        const val SEED_SIZE = 32
        const val PUB_SIZE = 32
    }
}
