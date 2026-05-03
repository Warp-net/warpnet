/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.di

import android.content.Context
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Status
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
import okhttp3.OkHttpClient
import site.warpnet.transport.Ed25519IdentityStore
import site.warpnet.transport.WarpnetClient
import site.warpnet.transport.WarpnetTransport

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
            Status.Visibility::class.java,
            EnumJsonAdapter.create(Status.Visibility::class.java)
                .withUnknownFallback(Status.Visibility.UNKNOWN),
        )
        .build()

    @Provides
    @Singleton
    fun providesWarpnetClient(moshi: Moshi): WarpnetClient =
        WarpnetTransport.createClient(moshi)

    @Provides
    @Singleton
    fun providesWarpnetIdentityStore(): Ed25519IdentityStore =
        WarpnetTransport.createIdentityStore()

    @Provides
    @Singleton
    fun providesPairedNodeStore(
        @ApplicationContext context: Context,
    ): PairedNodeStore = PairedNodeStore(context)

    // Kept for call-site compatibility: the deleted NetworkModule used to
    // provide this for WarpdroidApplication and PlayerModule. Media playback
    // isn't wired to Warpnet yet, so a vanilla client with no interceptors
    // is enough to keep the Hilt graph complete.
    @Provides
    @Singleton
    fun providesOkHttpClient(): OkHttpClient = OkHttpClient.Builder().build()
}
