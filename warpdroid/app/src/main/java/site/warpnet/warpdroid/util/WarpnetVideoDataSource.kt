/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.util

import android.net.Uri
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.BaseDataSource
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import java.io.IOException
import kotlin.math.min
import kotlinx.coroutines.runBlocking
import site.warpnet.warpdroid.warpnet.WarpnetRepository

private const val WARPNET_VIDEO_PREFIX = "warpnet://video/"

internal data class WarpnetVideoRef(val userId: String, val key: String) {
    companion object {
        fun parse(model: String): WarpnetVideoRef? {
            if (!model.startsWith(WARPNET_VIDEO_PREFIX)) return null
            val tail = model.removePrefix(WARPNET_VIDEO_PREFIX)
            val slash = tail.indexOf('/')
            if (slash <= 0 || slash >= tail.length - 1) return null
            val userId = tail.substring(0, slash)
            val key = tail.substring(slash + 1)
            if (userId.isBlank() || key.isBlank()) return null
            return WarpnetVideoRef(userId, key)
        }
    }
}

@OptIn(UnstableApi::class)
class WarpnetVideoDataSource(
    private val repo: WarpnetRepository,
) : BaseDataSource(/* isNetwork = */ true) {

    private var data: ByteArray? = null
    private var uri: Uri? = null
    private var readPosition: Int = 0
    private var bytesRemaining: Int = 0
    private var opened: Boolean = false

    override fun open(dataSpec: DataSpec): Long {
        transferInitializing(dataSpec)
        val ref = WarpnetVideoRef.parse(dataSpec.uri.toString())
            ?: throw IOException("not a warpnet video uri: ${dataSpec.uri}")

        val bytes = try {
            runBlocking { repo.getVideoBytes(ref.userId, ref.key) }
        } catch (e: Exception) {
            throw IOException("warpnet video ${ref.key} could not be fetched", e)
        } ?: throw IOException("warpnet video not found: ${ref.key}")

        if (dataSpec.position > bytes.size) {
            throw IOException("warpnet video ${ref.key}: position past the end")
        }

        data = bytes
        uri = dataSpec.uri
        readPosition = dataSpec.position.toInt()
        val available = bytes.size - readPosition
        bytesRemaining = if (dataSpec.length == C.LENGTH_UNSET.toLong()) {
            available
        } else {
            min(dataSpec.length.toInt(), available)
        }

        opened = true
        transferStarted(dataSpec)
        return bytesRemaining.toLong()
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (length == 0) return 0
        if (bytesRemaining == 0) return C.RESULT_END_OF_INPUT

        val toRead = min(length, bytesRemaining)
        System.arraycopy(requireNotNull(data), readPosition, buffer, offset, toRead)
        readPosition += toRead
        bytesRemaining -= toRead
        bytesTransferred(toRead)
        return toRead
    }

    override fun getUri(): Uri? = uri

    override fun close() {
        data = null
        uri = null
        if (opened) {
            opened = false
            transferEnded()
        }
    }

    @OptIn(UnstableApi::class)
    class Dispatching(
        private val warpnet: WarpnetVideoDataSource,
        private val fallback: DataSource,
    ) : DataSource {

        private var active: DataSource = fallback

        override fun open(dataSpec: DataSpec): Long {
            active = if (dataSpec.uri.toString().startsWith(WARPNET_VIDEO_PREFIX)) warpnet else fallback
            return active.open(dataSpec)
        }

        override fun read(buffer: ByteArray, offset: Int, length: Int): Int =
            active.read(buffer, offset, length)

        override fun getUri(): Uri? = active.uri

        override fun getResponseHeaders(): Map<String, List<String>> = active.responseHeaders

        override fun addTransferListener(transferListener: androidx.media3.datasource.TransferListener) {
            warpnet.addTransferListener(transferListener)
            fallback.addTransferListener(transferListener)
        }

        override fun close() = active.close()
    }

    @OptIn(UnstableApi::class)
    class Factory(
        private val fallback: DataSource.Factory,
        private val repo: WarpnetRepository,
    ) : DataSource.Factory {
        override fun createDataSource(): DataSource =
            Dispatching(WarpnetVideoDataSource(repo), fallback.createDataSource())
    }
}
