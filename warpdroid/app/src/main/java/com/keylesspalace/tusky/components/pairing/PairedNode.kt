/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import com.squareup.moshi.JsonClass
import site.warpnet.transport.dto.AuthNodeInfo

/**
 * Persisted shape for a successful pairing. Carries the credentials
 * needed to re-assert the paired token plus the pinned peer ID and
 * addresses required to reconnect without re-scanning. The pin is the
 * peer ID the user confirmed on screen; on every reconnect we verify the
 * libp2p handshake produced that same ID before trusting the connection.
 *
 * Mirrors the flat `warpnet/domain/warpnet.go::AuthNodeInfo`.
 */
@JsonClass(generateAdapter = true)
data class PairedNode(
    val token: String,
    val psk: String,
    val userId: String,
    val pinnedPeerId: String,
    val addresses: List<String>,
    val network: String,
    val bootstrapAddrs: List<String>,
) {
    companion object {
        fun from(info: AuthNodeInfo): PairedNode = PairedNode(
            token = info.token,
            psk = info.psk,
            userId = info.userId,
            pinnedPeerId = info.nodeId,
            addresses = info.addresses,
            network = info.network,
            bootstrapAddrs = info.bootstrapPeers,
        )
    }
}
