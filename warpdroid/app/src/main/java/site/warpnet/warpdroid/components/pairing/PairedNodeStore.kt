/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.atomic.AtomicReference
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Persists the raw QR pairing payload in Android Keystore-backed
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
 * degrades to in-memory only — auto re-auth on cold start is lost,
 * but the app stays usable.
 */
@Singleton
class PairedNodeStore @Inject constructor(
    @ApplicationContext private val context: Context,
) {
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
    }.onFailure { Log.w(TAG, "EncryptedSharedPreferences open failed", it) }
        .getOrNull()

    fun load(): PairedNode? = ref.get()

    fun save(node: PairedNode, rawQrJson: String) {
        Log.i(
            TAG,
            "save: pinnedPeer=${node.pinnedPeerId} userId=${node.userId} " +
                "addresses (n=${node.addresses.size}): ${node.addresses}",
        )
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
                Log.w(TAG, "loadRawQr decrypt failed; clearing", it)
                runCatching { handle.edit().remove(KEY_RAW_QR).apply() }
            }
            .getOrNull()
    }

    /** "Forget this node" — invoked from Settings and on failed re-auth. */
    fun clear() {
        ref.set(null)
        prefs?.edit()?.remove(KEY_RAW_QR)?.apply()
    }

    private companion object {
        const val PREFS_FILE = "warpnet_pairing_v2"
        const val LEGACY_PREFS_FILE = "warpnet_pairing"
        const val KEY_RAW_QR = "paired_fat_node_qr"
        const val TAG = "PairedNodeStore"
    }
}
