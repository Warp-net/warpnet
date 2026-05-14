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
        ref.set(node)
        prefs?.edit()?.putString(KEY_RAW_QR, rawQrJson)?.apply()
    }

    /** Returns the raw QR JSON payload persisted on the last successful pair, or null. */
    fun loadRawQr(): String? = prefs?.getString(KEY_RAW_QR, null)

    /** "Forget this node" — invoked from Settings and on failed re-auth. */
    fun clear() {
        ref.set(null)
        prefs?.edit()?.remove(KEY_RAW_QR)?.apply()
    }

    private companion object {
        const val PREFS_FILE = "warpnet_pairing"
        const val KEY_RAW_QR = "paired_fat_node_qr"
        const val TAG = "PairedNodeStore"
    }
}
