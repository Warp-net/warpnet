/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.transport

import com.squareup.moshi.JsonClass
import java.util.UUID

/**
 * Wire envelope wrapping every Warpnet request and response.
 *
 * Matches `event.Message` from warpnet/event/event.go:250. Field names are the
 * JSON wire names; the server side uses `json-iterator/go` with default tag
 * handling, so numeric vs. string vs. time-RFC3339 encodings must match Go
 * stdlib defaults.
 *
 * `body` is the raw JSON of the inner event payload (e.g. GetUserEvent,
 * NewTweetEvent). It is transmitted verbatim so the signature verifies
 * byte-for-byte against what the server reads.
 */
@JsonClass(generateAdapter = true)
data class WarpnetEnvelope(
    val body: String,
    val message_id: String,
    val node_id: String,
    val path: String,
    val timestamp: String,
    val version: String,
    val signature: String,
) {
    companion object {
        const val VERSION = "0.0.0"

        /**
         * Build an envelope with a random message_id and current timestamp.
         * The caller must still sign the body and set `signature` before
         * putting the envelope on the wire.
         */
        fun unsigned(
            body: String,
            nodeId: String,
            path: String,
            timestamp: String,
        ): WarpnetEnvelope = WarpnetEnvelope(
            body = body,
            message_id = UUID.randomUUID().toString(),
            node_id = nodeId,
            path = path,
            timestamp = timestamp,
            version = VERSION,
            signature = "",
        )
    }
}

/**
 * Shape of a server error response. `warpnet/event/event.go` returns this
 * encoded as JSON when a handler fails, instead of the normal response type.
 */
@JsonClass(generateAdapter = true)
data class WarpnetResponseError(
    val code: Int,
    val message: String,
)
