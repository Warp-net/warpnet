/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import android.content.Context
import android.content.SharedPreferences
import android.util.Base64
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import dagger.hilt.android.qualifiers.ApplicationContext
import java.security.SecureRandom
import java.util.concurrent.atomic.AtomicReference
import javax.inject.Inject
import javax.inject.Singleton
import site.warpnet.transport.IdentitySeedStore
import timber.log.Timber

/**
 * Persists the pairing material — the raw QR payload and the seed of the
 * libp2p identity the paired node authorizes — in Android Keystore-backed
 * EncryptedSharedPreferences so the app can re-authenticate after a
 * cold start without forcing the user to re-scan. The parsed
 * [PairedNode] itself is held in memory for the lifetime of the
 * process — every cold start re-derives it from the stored QR JSON
 * via [PairingCoordinator].
 *
 * The encrypted prefs are opened lazily and behind a recovery path:
 * an unreadable keyset (KeyStore key invalidated, prefs file corrupted)
 * triggers a one-shot delete-and-reopen so the user lands on a clean
 * scanner instead of a crash loop. If even the retry fails the store
 * degrades to plain app-private prefs for the identity seed — a private
 * file still beats an identity anyone could recompute — and to in-memory
 * only for the QR: auto re-auth on cold start is lost, but the app stays
 * usable.
 */
@Singleton
class PairedNodeStore @Inject constructor(
    @ApplicationContext private val context: Context,
) : IdentitySeedStore {
    private val ref = AtomicReference<PairedNode?>(null)

    private val prefs: SharedPreferences? by lazy(LazyThreadSafetyMode.SYNCHRONIZED) {
        // The previous on-disk pairing schema lived in `warpnet_pairing`
        // and used a different value layout. Wipe it on first access of
        // the new file so legacy encrypted entries don't linger after an
        // upgrade past the in-memory-only build.
        runCatching { context.deleteSharedPreferences(LEGACY_PREFS_FILE) }
        openPrefs() ?: run {
            // Best-effort wipe and retry once. The most common failure mode
            // is a KeyStore key invalidated by a lock-screen-credential
            // reset, which leaves the keyset header undecryptable; deleting
            // the prefs file lets MasterKey rebuild from scratch.
            context.deleteSharedPreferences(PREFS_FILE)
            openPrefs()
        }
    }

    private fun openPrefs(): SharedPreferences? = runCatching {
        EncryptedSharedPreferences.create(
            context,
            PREFS_FILE,
            MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build(),
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }.onFailure { Timber.tag(TAG).w(it, "EncryptedSharedPreferences open failed") }
        .getOrNull()

    fun load(): PairedNode? = ref.get()

    fun save(node: PairedNode, rawQrJson: String) {
        Timber.tag(TAG).i("save: pinnedPeer=${node.pinnedPeerId} userId=${node.userId} " +
                "addresses (n=${node.addresses.size}): ${node.addresses}")
        ref.set(node)
        prefs?.edit()?.putString(KEY_RAW_QR, rawQrJson)?.apply()
    }

    /**
     * Returns the raw QR JSON payload persisted on the last successful pair,
     * or null when nothing is stored or decryption fails. A decrypt failure
     * means the keyset got out of sync with the stored value (the prefs file
     * survived a key-invalidating event the [openPrefs] retry path didn't
     * catch); wipe the entry so the next launch lands on a clean scanner.
     */
    fun loadRawQr(): String? {
        val handle = prefs ?: return null
        return runCatching { handle.getString(KEY_RAW_QR, null) }
            .onFailure {
                Timber.tag(TAG).w(it, "loadRawQr decrypt failed; clearing")
                runCatching { handle.edit().remove(KEY_RAW_QR).apply() }
            }
            .getOrNull()
    }

    /**
     * The identity seed for the pairing with [memberPeerId], drawn from
     * [SecureRandom] the first time that node is paired with. Reading or
     * creating it touches disk, so callers stay off the main thread.
     *
     * The seed is scoped to one member node: pairing with a different one
     * retires the previous identity instead of letting a single key follow
     * the device between nodes.
     */
    @Synchronized
    override fun seed(memberPeerId: String): ByteArray {
        val handle = seedPrefs
        val stored = runCatching {
            if (handle.getString(KEY_IDENTITY_NODE, null) != memberPeerId) {
                null
            } else {
                handle.getString(KEY_IDENTITY_SEED, null)
            }
        }.getOrNull()

        if (stored != null) {
            val decoded = runCatching { Base64.decode(stored, Base64.NO_WRAP) }.getOrNull()
            if (decoded != null && decoded.size == SEED_SIZE) return decoded
            Timber.tag(TAG).w("stored identity seed is unusable; generating a new one")
        }

        val seed = ByteArray(SEED_SIZE).also(SecureRandom()::nextBytes)
        runCatching {
            handle.edit()
                .putString(KEY_IDENTITY_NODE, memberPeerId)
                .putString(KEY_IDENTITY_SEED, Base64.encodeToString(seed, Base64.NO_WRAP))
                .commit()
        }.onFailure {
            // An identity that cannot be stored still pairs, but the next
            // cold start draws another one and has to pair again.
            Timber.tag(TAG).w(it, "identity seed not persisted")
        }
        return seed
    }

    /**
     * "Forget this node" — invoked from Settings and on failed re-auth.
     * Drops the identity along with the QR: the seed is what the paired
     * node authorizes, so leaving it behind would keep the device's key
     * alive after the user asked for it to be forgotten.
     */
    fun clear() {
        ref.set(null)
        prefs?.edit()?.remove(KEY_RAW_QR)?.apply()
        // Both stores: a degraded session may have left a seed in the plain
        // file even if the encrypted one opens today.
        runCatching {
            prefs?.edit()?.remove(KEY_IDENTITY_NODE)?.remove(KEY_IDENTITY_SEED)?.apply()
        }
        runCatching {
            fallbackSeedPrefs.edit().remove(KEY_IDENTITY_NODE).remove(KEY_IDENTITY_SEED).apply()
        }
    }

    // The encrypted store when it opens, a plain app-private file when it
    // does not. Both are readable only by this app; neither is reproducible
    // from outside it.
    private val seedPrefs: SharedPreferences
        get() = prefs ?: fallbackSeedPrefs

    private val fallbackSeedPrefs: SharedPreferences by lazy(LazyThreadSafetyMode.SYNCHRONIZED) {
        context.getSharedPreferences(SEED_FALLBACK_PREFS_FILE, Context.MODE_PRIVATE)
    }

    private companion object {
        const val PREFS_FILE = "warpnet_pairing_v2"
        const val LEGACY_PREFS_FILE = "warpnet_pairing"
        const val SEED_FALLBACK_PREFS_FILE = "warpnet_identity"
        const val KEY_RAW_QR = "paired_fat_node_qr"
        const val KEY_IDENTITY_NODE = "identity_member_node"
        const val KEY_IDENTITY_SEED = "identity_seed"
        const val SEED_SIZE = 32
        const val TAG = "PairedNodeStore"
    }
}
