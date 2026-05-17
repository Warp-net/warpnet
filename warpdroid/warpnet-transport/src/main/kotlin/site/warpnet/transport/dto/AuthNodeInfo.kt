/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport.dto

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Wire type for the QR pairing handshake. Field names mirror
 * `warpnet/domain/warpnet.go::AuthNodeInfo` byte-for-byte so a round-trip
 * back to the fat node reproduces whatever the node originally embedded
 * in the QR.
 */

@JsonClass(generateAdapter = true)
data class AuthNodeInfo(
    @Json(name = "token") val token: String,
    @Json(name = "psk") val psk: String,
    @Json(name = "network") val network: String = "",
    @Json(name = "addresses") val addresses: List<String> = emptyList(),
    @Json(name = "node_id") val nodeId: String,
    @Json(name = "user_id") val userId: String,
    @Json(name = "bootstrap_peers") val bootstrapPeers: List<String> = emptyList(),
)
