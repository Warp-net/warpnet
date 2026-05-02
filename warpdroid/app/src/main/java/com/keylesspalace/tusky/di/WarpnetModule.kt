/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package com.keylesspalace.tusky.di

import android.content.Context
import com.keylesspalace.tusky.components.pairing.PairedNodeStore
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.entity.Notification
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.json.GuardedAdapter
import com.keylesspalace.tusky.json.NotificationTypeAdapter
import com.keylesspalace.tusky.json.StringOrBooleanAdapter
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
 * DI wiring for the Warpnet transport + mapper stack. The previous Mastodon
 * [com.keylesspalace.tusky.di.NetworkModule] is gone; Moshi is provided here
 * because [com.keylesspalace.tusky.warpnet.WarpnetRepository] and the rest of
 * the app still need Mastodon-shaped JSON adapters for the entity types we
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
    fun providesWarpnetIdentityStore(
        @ApplicationContext context: Context,
    ): Ed25519IdentityStore = WarpnetTransport.createIdentityStore(context.filesDir)

    @Provides
    @Singleton
    fun providesPairedNodeStore(
        @ApplicationContext context: Context,
        moshi: Moshi,
    ): PairedNodeStore = PairedNodeStore(context, moshi)

    // Kept for call-site compatibility: the deleted NetworkModule used to
    // provide this for TuskyApplication and PlayerModule. Media playback
    // isn't wired to Warpnet yet, so a vanilla client with no interceptors
    // is enough to keep the Hilt graph complete.
    @Provides
    @Singleton
    fun providesOkHttpClient(): OkHttpClient = OkHttpClient.Builder().build()
}
