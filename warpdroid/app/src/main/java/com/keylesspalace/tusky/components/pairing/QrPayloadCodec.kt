/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.components.pairing

import java.io.ByteArrayInputStream
import java.util.zip.GZIPInputStream

/**
 * Decodes the pairing payload encoded by the desktop frontend: gzip-
 * compressed JSON (RFC 1952), Base45-encoded for QR alphanumeric mode.
 *
 * The frontend gzip-compresses the AuthNodeInfo JSON at level 9 and
 * encodes the result with Base45 (RFC 9285). The Base45 alphabet is a
 * strict subset of QR alphanumeric mode, which packs 5.5 bits per
 * character versus the 8 bits-per-character of byte mode and lets the
 * full envelope fit without trimming any dial fields.
 */
object QrPayloadCodec {

    /**
     * Decodes [payload] back into the original UTF-8 JSON. The raw QR text
     * is first attempted; only Base45-shaped strings are decompressed, so
     * legacy plain-JSON QR codes (pre-compression rollout) still validate
     * cleanly through the same caller.
     */
    fun decode(payload: String): String {
        val trimmed = payload.trim()
        if (trimmed.isEmpty()) return ""
        // Plain JSON starts with '{'; let the caller handle it directly.
        if (trimmed.startsWith("{")) return trimmed
        val compressed = Base45.decode(trimmed)
        GZIPInputStream(ByteArrayInputStream(compressed)).use { input ->
            return String(input.readBytes(), Charsets.UTF_8)
        }
    }
}

/**
 * Base45 codec per RFC 9285. Implemented inline rather than pulled in as a
 * dependency so the transport module doesn't grow another transitive for
 * what is ~50 lines of arithmetic.
 */
internal object Base45 {
    private const val ALPHABET = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ \$%*+-./:"

    private val DECODE_TABLE = IntArray(256) { -1 }.also { table ->
        for (i in ALPHABET.indices) {
            table[ALPHABET[i].code] = i
        }
    }

    fun encode(data: ByteArray): String {
        if (data.isEmpty()) return ""
        val sb = StringBuilder((data.size / 2) * 3 + 2)
        var i = 0
        while (i + 1 < data.size) {
            val v = ((data[i].toInt() and 0xFF) shl 8) or (data[i + 1].toInt() and 0xFF)
            val e = v / (45 * 45)
            val r = v % (45 * 45)
            val d = r / 45
            val c = r % 45
            sb.append(ALPHABET[c]).append(ALPHABET[d]).append(ALPHABET[e])
            i += 2
        }
        if (i < data.size) {
            val v = data[i].toInt() and 0xFF
            val d = v / 45
            val c = v % 45
            sb.append(ALPHABET[c]).append(ALPHABET[d])
        }
        return sb.toString()
    }

    fun decode(s: String): ByteArray {
        if (s.isEmpty()) return ByteArray(0)
        val n = s.length
        val rem = n % 3
        require(rem != 1) { "base45: invalid length" }
        val full = n / 3
        val out = ByteArray(full * 2 + rem / 2)
        var p = 0
        for (i in 0 until full) {
            val c = lookup(s[i * 3])
            val d = lookup(s[i * 3 + 1])
            val e = lookup(s[i * 3 + 2])
            val v = c + d * 45 + e * 45 * 45
            require(v <= 0xFFFF) { "base45: value out of range" }
            out[p++] = (v ushr 8).toByte()
            out[p++] = (v and 0xFF).toByte()
        }
        if (rem == 2) {
            val c = lookup(s[full * 3])
            val d = lookup(s[full * 3 + 1])
            val v = c + d * 45
            require(v <= 0xFF) { "base45: value out of range" }
            out[p] = v.toByte()
        }
        return out
    }

    private fun lookup(ch: Char): Int {
        val code = ch.code
        require(code in 0..255) { "base45: invalid character" }
        val v = DECODE_TABLE[code]
        require(v >= 0) { "base45: invalid character" }
        return v
    }
}
