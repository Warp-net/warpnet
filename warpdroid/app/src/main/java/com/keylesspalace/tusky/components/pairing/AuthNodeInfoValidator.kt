/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import com.squareup.moshi.JsonDataException
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapter
import site.warpnet.transport.dto.AuthNodeInfo

/**
 * Validates the JSON decoded from a scanned QR. The raw bytes are kept
 * alongside the parsed DTO so the pairing handshake can echo them back to
 * the fat node byte-for-byte — avoiding any re-encoding drift that might
 * break the token comparison in `warpnet/core/handler/pair.go`.
 *
 * The fat node ships the QR payload Brotli-compressed and Base45-encoded
 * (see `warpnet/security/qrpayload.go`); [QrPayloadCodec] reverses that
 * before the JSON parse. Plain-JSON payloads (legacy / manual paste) are
 * passed through unchanged by the codec.
 *
 * Hash is deliberately NOT verified here (per pairing spec).
 */
sealed class ValidationResult {
    data class Valid(
        val authNodeInfo: AuthNodeInfo,
        val rawJson: String,
    ) : ValidationResult()

    data class Invalid(val reason: String) : ValidationResult()
}

@OptIn(ExperimentalStdlibApi::class)
class AuthNodeInfoValidator(moshi: Moshi) {
    private val adapter = moshi.adapter<AuthNodeInfo>()

    fun validate(rawPayload: String): ValidationResult {
        if (rawPayload.isBlank()) return ValidationResult.Invalid("empty QR payload")
        val rawJson = try {
            QrPayloadCodec.decode(rawPayload)
        } catch (e: IllegalArgumentException) {
            return ValidationResult.Invalid("malformed pairing payload: ${e.message}")
        } catch (e: java.io.IOException) {
            return ValidationResult.Invalid("malformed pairing payload: ${e.message}")
        }
        if (rawJson.isBlank()) return ValidationResult.Invalid("empty pairing payload")
        val parsed = try {
            adapter.fromJson(rawJson)
        } catch (e: JsonDataException) {
            return ValidationResult.Invalid("malformed pairing JSON: ${e.message}")
        } catch (e: java.io.IOException) {
            return ValidationResult.Invalid("malformed pairing JSON: ${e.message}")
        } ?: return ValidationResult.Invalid("empty pairing JSON")

        val missing = mutableListOf<String>()
        if (parsed.token.isBlank()) missing += "identity.token"
        if (parsed.psk.isBlank()) missing += "identity.psk"
        if (parsed.nodeId.isBlank()) missing += "identity.owner.node_id"
        if (parsed.userId.isBlank()) missing += "identity.owner.user_id"
        if (parsed.addresses.isEmpty()) missing += "node_info.addresses"
        if (parsed.network.isBlank()) missing += "node_info.network"
        if (missing.isNotEmpty()) {
            return ValidationResult.Invalid("missing fields: ${missing.joinToString()}")
        }

        // A syntactically valid multiaddr must start with "/" and have at
        // least one segment. go-libp2p's parser does the final check on the
        // desktop side; this is the minimum-viable prefilter so we don't
        // even bother dialling obvious garbage.
        val hasDialable = parsed.addresses.any {
            it.startsWith("/") && it.trim('/').isNotEmpty()
        }
        if (!hasDialable) {
            return ValidationResult.Invalid("no parseable multiaddr in node_info.addresses")
        }

        return ValidationResult.Valid(parsed, rawJson)
    }
}
