/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.google.crypto.tink.subtle.Ed25519Sign
import java.io.File

/**
 * Loads or generates the 64-byte libp2p Ed25519 private key that the AAR's
 * `Initialize` method consumes. The key is `seed(32) || publicKey(32)`, as
 * expected by `crypto.UnmarshalEd25519PrivateKey` in go-libp2p.
 *
 * Persistence lives under the app's private files directory; there is no
 * encryption at rest because the key is already the user's long-term libp2p
 * identity and the file lives in app-private storage. If a pairing flow
 * later demands hardware-backed protection, move the read/write calls onto
 * EncryptedFile without changing the public signature here.
 */
class Ed25519IdentityStore(private val keyFile: File) {

    /** Read from disk or generate on first use. Returns 64 raw bytes. */
    fun loadOrCreate(): ByteArray {
        existing()?.let { return it }
        val fresh = generate()
        writeAtomically(fresh)
        return fresh
    }

    /**
     * Write to a sibling temp file, fsync, then rename onto [keyFile]. This
     * avoids leaving a half-written identity that would silently rotate the
     * peer ID on next launch if the process is killed mid-write.
     */
    private fun writeAtomically(bytes: ByteArray) {
        val parent = keyFile.parentFile
        parent?.mkdirs()
        val tmp = File.createTempFile(keyFile.name, ".tmp", parent)
        try {
            java.io.FileOutputStream(tmp).use { out ->
                out.write(bytes)
                out.fd.sync()
            }
            if (!tmp.renameTo(keyFile)) {
                throw java.io.IOException("failed to rename ${tmp.absolutePath} -> ${keyFile.absolutePath}")
            }
        } catch (t: Throwable) {
            tmp.delete()
            throw t
        }
    }

    /** Same as [loadOrCreate] but hex-encoded (lowercase), ready for the AAR. */
    @OptIn(ExperimentalStdlibApi::class)
    fun loadOrCreateHex(): String = loadOrCreate().toHexString()

    private fun existing(): ByteArray? {
        if (!keyFile.exists()) return null
        val bytes = keyFile.readBytes()
        return if (bytes.size == KEY_SIZE) bytes else null
    }

    private fun generate(): ByteArray {
        // Conscrypt 2.5.x ships an Ed25519 KeyFactory but no
        // KeyPairGenerator, and the platform AndroidOpenSSL provider does
        // not register Ed25519 at all on min SDK 24, so going through JCE
        // hard-fails with NoSuchAlgorithmException on real devices. Tink's
        // standalone Ed25519 implementation does not need a JCE provider
        // and yields the raw 32-byte seed and public key go-libp2p expects.
        val kp = Ed25519Sign.KeyPair.newKeyPair()
        val seed = kp.privateKey
        val pub = kp.publicKey
        require(seed.size == SEED_SIZE && pub.size == PUB_SIZE) {
            "Unexpected Ed25519 key lengths (seed=${seed.size}, pub=${pub.size})"
        }
        return seed + pub
    }

    private companion object {
        const val SEED_SIZE = 32
        const val PUB_SIZE = 32
        const val KEY_SIZE = SEED_SIZE + PUB_SIZE
    }
}
