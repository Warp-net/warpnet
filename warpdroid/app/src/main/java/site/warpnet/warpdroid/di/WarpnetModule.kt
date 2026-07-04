/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.di

import android.content.Context
import site.warpnet.warpdroid.components.logviewer.LogBuffer
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.json.GuardedAdapter
import site.warpnet.warpdroid.json.NotificationTypeAdapter
import site.warpnet.warpdroid.json.StringOrBooleanAdapter
import com.squareup.moshi.Moshi
import com.squareup.moshi.adapters.EnumJsonAdapter
import com.squareup.moshi.adapters.Rfc3339DateJsonAdapter
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.util.Date
import javax.inject.Singleton
import kotlinx.coroutines.CoroutineScope
import site.warpnet.transport.ConnectionMonitor
import site.warpnet.transport.Ed25519IdentityStore
import site.warpnet.transport.WarpnetClient
import site.warpnet.transport.WarpnetTransport
import timber.log.Timber

/**
 * DI wiring for the Warpnet transport + mapper stack. The previous Warpnet
 * [site.warpnet.warpdroid.di.NetworkModule] is gone; Moshi is provided here
 * because [site.warpnet.warpdroid.warpnet.WarpnetRepository] and the rest of
 * the app still need Warpnet-shaped JSON adapters for the entity types we
 * kept.
 */
@Module
@InstallIn(SingletonComponent::class)
object WarpnetModule {

    @Provides
    @Singleton
    fun providesMoshi(): Moshi = Moshi.Builder()
        .add(GuardedAdapter.ANNOTATION_FACTORY)
        .add(StringOrBooleanAdapter.ANNOTATION_FACTORY)
        .add(Date::class.java, Rfc3339DateJsonAdapter())
        .add(
            Attachment.Type::class.java,
            EnumJsonAdapter.create(Attachment.Type::class.java)
                .withUnknownFallback(Attachment.Type.UNKNOWN),
        )
        .add(Notification.Type::class.java, NotificationTypeAdapter())
        .add(
            Tweet.Visibility::class.java,
            EnumJsonAdapter.create(Tweet.Visibility::class.java)
                .withUnknownFallback(Tweet.Visibility.UNKNOWN),
        )
        .build()

    @Provides
    @Singleton
    fun providesWarpnetClient(moshi: Moshi, logBuffer: LogBuffer): WarpnetClient =
        WarpnetTransport.createClient(moshi, nodeLogSink = logBuffer.goSink)

    @Provides
    @Singleton
    fun providesWarpnetIdentityStore(): Ed25519IdentityStore =
        WarpnetTransport.createIdentityStore()

    @Provides
    @Singleton
    fun providesPairedNodeStore(
        @ApplicationContext context: Context,
    ): PairedNodeStore = PairedNodeStore(context)

    /**
     * Single live ConnectionMonitor for the process. Dial candidates are
     * sourced lazily from [PairedNodeStore] so a re-pair propagates
     * automatically — the closure captures the store by reference, not
     * its value at injection time.
     *
     * The dial-candidate list is logged on every reconnect attempt so
     * stale / missing LAN addresses are visible in logcat without
     * needing to inspect SharedPreferences. Tag `warpnet-dial`.
     */
    @Provides
    @Singleton
    fun providesConnectionMonitor(
        client: WarpnetClient,
        pairedNodeStore: PairedNodeStore,
        @ApplicationScope scope: CoroutineScope,
    ): ConnectionMonitor = WarpnetTransport.createConnectionMonitor(
        client = client,
        scope = scope,
        dialAddresses = {
            val candidates = pairedNodeStore.load()?.let { paired ->
                paired.addresses.map { "$it/p2p/${paired.pinnedPeerId}" }
            } ?: emptyList()
            Timber.tag("warpnet-dial").i("dial candidates (n=${candidates.size}): $candidates")
            candidates
        },
    )
}
