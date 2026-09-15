/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.Moshi
import kotlinx.coroutines.CoroutineScope

/**
 * Hilt module lives in the `app` module so it can bind app-scoped types. This
 * factory exists to keep knowledge of transport wiring out of the app code —
 * the module only has to know "call [createClient]" with a [Moshi] it owns.
 */
object WarpnetTransport {
    fun createClient(
        moshi: Moshi = Moshi.Builder().build(),
        nodeLogSink: NodeLogSink? = null,
    ): WarpnetClient = WarpnetClient(
        moshi = moshi,
        signer = BindingSigner(DefaultBinding),
        nodeLogSink = nodeLogSink,
    )

    /**
     * Build an [Ed25519IdentityStore] over the app's seed storage. The
     * identity is random per pairing, so it only exists as long as [seeds]
     * keeps it.
     */
    fun createIdentityStore(seeds: IdentitySeedStore): Ed25519IdentityStore =
        Ed25519IdentityStore(seeds)

    /**
     * Build the connection monitor that polls the live link state and
     * runs the reconnect loop. [dialAddresses] is wired by the app
     * layer so the transport module stays independent of any concrete
     * paired-node store.
     */
    fun createConnectionMonitor(
        client: WarpnetClient,
        scope: CoroutineScope,
        dialAddresses: suspend () -> List<String>,
    ): ConnectionMonitor = ConnectionMonitor(
        client = client,
        binding = DefaultBinding,
        dialAddresses = dialAddresses,
        parentScope = scope,
    )
}
