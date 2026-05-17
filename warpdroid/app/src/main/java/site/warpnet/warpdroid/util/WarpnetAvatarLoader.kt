/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Glide ModelLoader that resolves the synthetic "warpnet://avatar/{userId}/{key}"
 * URLs emitted by [WarpnetMapper] into image bytes fetched via
 * [WarpnetRepository.getImageBytes]. Registering it once in [GlideModule]
 * means every existing call site — drawer header, account screen, timeline
 * cards, notification rows, fullscreen viewer — picks up Warpnet avatars
 * without local edits; the only requirement is that the URL string carries
 * the userId + key pair.
 */
package site.warpnet.warpdroid.util

import android.content.Context
import com.bumptech.glide.Priority
import com.bumptech.glide.load.DataSource
import com.bumptech.glide.load.Options
import com.bumptech.glide.load.data.DataFetcher
import com.bumptech.glide.load.model.ModelLoader
import com.bumptech.glide.load.model.ModelLoaderFactory
import com.bumptech.glide.load.model.MultiModelLoaderFactory
import com.bumptech.glide.signature.ObjectKey
import dagger.hilt.EntryPoint
import dagger.hilt.EntryPoints
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import java.nio.ByteBuffer
import kotlinx.coroutines.runBlocking
import site.warpnet.warpdroid.warpnet.WarpnetRepository

/** Marker prefix the mapper emits; [WarpnetMapper.warpnetImageUrl]. */
private const val WARPNET_AVATAR_PREFIX = "warpnet://avatar/"

@EntryPoint
@InstallIn(SingletonComponent::class)
interface WarpnetGlideEntryPoint {
    fun warpnetRepository(): WarpnetRepository
}

/** Pulled out so [WarpnetAvatarLoader] and tests can share the same parser. */
internal data class WarpnetAvatarRef(val userId: String, val key: String) {
    companion object {
        fun parse(model: String): WarpnetAvatarRef? {
            if (!model.startsWith(WARPNET_AVATAR_PREFIX)) return null
            val tail = model.removePrefix(WARPNET_AVATAR_PREFIX)
            val slash = tail.indexOf('/')
            if (slash <= 0 || slash >= tail.length - 1) return null
            val userId = tail.substring(0, slash)
            val key = tail.substring(slash + 1)
            if (userId.isBlank() || key.isBlank()) return null
            return WarpnetAvatarRef(userId, key)
        }
    }
}

class WarpnetAvatarLoader(
    private val repo: WarpnetRepository,
) : ModelLoader<String, ByteBuffer> {

    override fun handles(model: String): Boolean = model.startsWith(WARPNET_AVATAR_PREFIX)

    override fun buildLoadData(
        model: String,
        width: Int,
        height: Int,
        options: Options,
    ): ModelLoader.LoadData<ByteBuffer>? {
        val ref = WarpnetAvatarRef.parse(model) ?: return null
        return ModelLoader.LoadData(ObjectKey(model), Fetcher(repo, ref))
    }

    private class Fetcher(
        private val repo: WarpnetRepository,
        private val ref: WarpnetAvatarRef,
    ) : DataFetcher<ByteBuffer> {

        override fun loadData(priority: Priority, callback: DataFetcher.DataCallback<in ByteBuffer>) {
            // Glide calls loadData on its background executor, so blocking
            // here is fine. runBlocking + a fresh suspending fetch is the
            // simplest bridge from Glide's callback API to the
            // coroutine-based repository.
            try {
                val bytes = runBlocking { repo.getImageBytes(ref.userId, ref.key) }
                if (bytes == null || bytes.isEmpty()) {
                    callback.onLoadFailed(NoSuchElementException("warpnet image not found: ${ref.key}"))
                } else {
                    callback.onDataReady(ByteBuffer.wrap(bytes))
                }
            } catch (t: Throwable) {
                callback.onLoadFailed(Exception(t))
            }
        }

        override fun cleanup() = Unit
        override fun cancel() = Unit
        override fun getDataClass(): Class<ByteBuffer> = ByteBuffer::class.java
        override fun getDataSource(): DataSource = DataSource.REMOTE
    }

    class Factory(
        private val repo: WarpnetRepository,
    ) : ModelLoaderFactory<String, ByteBuffer> {
        override fun build(multiFactory: MultiModelLoaderFactory): ModelLoader<String, ByteBuffer> =
            WarpnetAvatarLoader(repo)

        override fun teardown() = Unit

        companion object {
            // Glide initialises the [com.bumptech.glide.module.AppGlideModule]
            // before Hilt's component graph is exposed to arbitrary callers,
            // but the application Context is enough to grab the singleton
            // EntryPoint, which Hilt installs eagerly during application
            // bootstrap. No injection magic needed inside Glide internals.
            fun forContext(context: Context): Factory {
                val ep = EntryPoints.get(
                    context.applicationContext,
                    WarpnetGlideEntryPoint::class.java,
                )
                return Factory(ep.warpnetRepository())
            }
        }
    }
}
