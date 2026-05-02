/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import java.io.File

/**
 * Hilt module lives in the `app` module so it can bind app-scoped types. This
 * factory exists to keep knowledge of transport wiring out of the app code —
 * the module only has to know "call [createClient]" with a [Moshi] it owns.
 */
object WarpnetTransport {
    /** Filename of the persisted 64-byte Ed25519 identity. */
    const val IDENTITY_FILENAME = "warpnet_identity.key"

    fun createClient(moshi: Moshi = Moshi.Builder().build()): WarpnetClient =
        WarpnetClient(moshi = moshi, signer = NoOpSigner())

    /**
     * Build an [Ed25519IdentityStore] anchored at the caller's app-private
     * files dir. The caller is responsible for passing
     * [android.content.Context.getFilesDir] or equivalent.
     */
    fun createIdentityStore(filesDir: File): Ed25519IdentityStore =
        Ed25519IdentityStore(File(filesDir, IDENTITY_FILENAME))
}
