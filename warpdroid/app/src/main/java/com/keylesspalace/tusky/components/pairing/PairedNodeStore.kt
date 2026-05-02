/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapter
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Keystore-backed persistence for the current pairing. One JSON blob under
 * [KEY_PAIRED_FAT_NODE] holds everything needed to reconnect — [Identity]
 * (token + PSK + owner metadata) plus the pinned peer ID and addresses.
 *
 * Token and PSK never leave this store unencrypted; the returned [PairedNode]
 * is held only in memory by the transport layer.
 */
@OptIn(ExperimentalStdlibApi::class)
@Singleton
class PairedNodeStore @Inject constructor(
    context: Context,
    moshi: Moshi,
) {
    private val adapter = moshi.adapter<PairedNode>()
    private val prefs: SharedPreferences = EncryptedSharedPreferences.create(
        context,
        PREFS_FILE,
        MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun load(): PairedNode? {
        val raw = prefs.getString(KEY_PAIRED_FAT_NODE, null) ?: return null
        return runCatching { adapter.fromJson(raw) }.getOrNull()
    }

    fun save(node: PairedNode) {
        prefs.edit().putString(KEY_PAIRED_FAT_NODE, adapter.toJson(node)).apply()
    }

    /** "Forget this node" — invoked from Settings. */
    fun clear() {
        prefs.edit().remove(KEY_PAIRED_FAT_NODE).apply()
    }

    companion object {
        const val KEY_PAIRED_FAT_NODE = "paired_fat_node"
        private const val PREFS_FILE = "warpnet_pairing"
    }
}
