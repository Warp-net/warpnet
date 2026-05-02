/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Originally part of Tusky. The drafts table no longer exists; sending
 * happens entirely over a libp2p stream and failure paths surface directly
 * from [site.warpnet.transport.WarpnetClient]. This stub keeps the DI
 * binding alive so activities that inject it still compile.
 */
package com.keylesspalace.tusky.db

import android.content.Context
import androidx.lifecycle.LifecycleOwner
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class DraftsAlert @Inject constructor() {
    fun <T> observeInContext(context: T, showAlert: Boolean) where T : Context, T : LifecycleOwner {
        // No-op: Warpdroid has no drafts persistence layer.
    }
}
