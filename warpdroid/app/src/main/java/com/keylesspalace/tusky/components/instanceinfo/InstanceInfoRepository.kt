/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Stub replacement for Tusky's Mastodon-instance probing. Warpnet has no
 * notion of per-instance character limits, upload ceilings, VAPID keys
 * etc.; all call sites get a constant [defaultInstanceInfo]. The Emoji
 * surface is empty because Warpnet has no custom-emoji concept either.
 */
package com.keylesspalace.tusky.components.instanceinfo

import com.keylesspalace.tusky.entity.Emoji
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

@Singleton
class InstanceInfoRepository @Inject constructor() {

    val defaultInstanceInfo: InstanceInfo = InstanceInfo(
        maxChars = DEFAULT_CHARACTER_LIMIT,
        pollMaxOptions = DEFAULT_MAX_OPTION_COUNT,
        pollMaxLength = DEFAULT_MAX_OPTION_LENGTH,
        pollMinDuration = DEFAULT_MIN_POLL_DURATION,
        pollMaxDuration = DEFAULT_MAX_POLL_DURATION,
        charactersReservedPerUrl = DEFAULT_CHARACTERS_RESERVED_PER_URL,
        videoSizeLimit = DEFAULT_VIDEO_SIZE_LIMIT,
        imageSizeLimit = DEFAULT_IMAGE_SIZE_LIMIT,
        imageMatrixLimit = DEFAULT_IMAGE_MATRIX_LIMIT,
        maxMediaAttachments = DEFAULT_MAX_MEDIA_ATTACHMENTS,
        mediaDescriptionLimit = DEFAULT_MEDIA_DESCRIPTION_LIMIT,
        maxFields = DEFAULT_MAX_ACCOUNT_FIELDS,
        maxFieldNameLength = null,
        maxFieldValueLength = null,
        version = null,
        translationEnabled = false,
        mastodonApiVersion = null,
        vapidKey = null,
    )

    private val flow = MutableStateFlow(defaultInstanceInfo)

    fun instanceInfoFlow(): Flow<InstanceInfo> = flow.asStateFlow()

    suspend fun getEmojis(): List<Emoji> = emptyList()

    suspend fun getUpdatedInstanceInfoOrFallback(): InstanceInfo = defaultInstanceInfo

    companion object {
        const val DEFAULT_CHARACTER_LIMIT = 500
        private const val DEFAULT_MAX_OPTION_COUNT = 4
        private const val DEFAULT_MAX_OPTION_LENGTH = 50
        private const val DEFAULT_MIN_POLL_DURATION = 300
        private const val DEFAULT_MAX_POLL_DURATION = 604800

        private const val DEFAULT_VIDEO_SIZE_LIMIT = 41943040
        private const val DEFAULT_IMAGE_SIZE_LIMIT = 10485760
        private const val DEFAULT_IMAGE_MATRIX_LIMIT = 16777216

        const val DEFAULT_MEDIA_DESCRIPTION_LIMIT = 1500
        const val DEFAULT_CHARACTERS_RESERVED_PER_URL = 23
        const val DEFAULT_MAX_MEDIA_ATTACHMENTS = 4
        const val DEFAULT_MAX_ACCOUNT_FIELDS = 4
    }
}
