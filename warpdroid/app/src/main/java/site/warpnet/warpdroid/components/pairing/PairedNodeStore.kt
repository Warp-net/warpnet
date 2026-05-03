/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.pairing

import android.content.Context
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.concurrent.atomic.AtomicReference
import javax.inject.Inject
import javax.inject.Singleton

/**
 * In-memory store for the active pairing.
 *
 * Auth state never touches disk: every cold start lands the user back at
 * the QR scanner. The previously-persisted EncryptedSharedPreferences
 * file is deleted on first construction so any pairing left over from
 * an older build of the app is wiped.
 */
@Singleton
class PairedNodeStore @Inject constructor(
    @ApplicationContext context: Context,
) {
    private val ref = AtomicReference<PairedNode?>(null)

    init {
        // Best-effort: drop the legacy encrypted-prefs file from earlier
        // builds. If anything in androidx.security failed to initialise
        // the master key, deleteSharedPreferences just no-ops.
        runCatching { context.deleteSharedPreferences(LEGACY_PREFS_FILE) }
    }

    fun load(): PairedNode? = ref.get()

    fun save(node: PairedNode) {
        ref.set(node)
    }

    /** "Forget this node" — invoked from Settings. */
    fun clear() {
        ref.set(null)
    }

    private companion object {
        const val LEGACY_PREFS_FILE = "warpnet_pairing"
    }
}
