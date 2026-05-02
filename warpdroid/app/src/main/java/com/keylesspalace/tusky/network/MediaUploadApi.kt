/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Warpnet has no media-upload RPC yet, so this is a plain Hilt-provided stub
 * that keeps [com.keylesspalace.tusky.components.compose.MediaUploader]
 * compiling. Every call fails with HTTP 501; the compose flow surfaces the
 * error to the user instead of silently dropping the attachment.
 */
package com.keylesspalace.tusky.network

import com.keylesspalace.tusky.entity.MediaUploadResult
import javax.inject.Inject
import javax.inject.Singleton
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.ResponseBody.Companion.toResponseBody
import retrofit2.Response

@Singleton
class MediaUploadApi @Inject constructor() {
    suspend fun uploadMedia(
        file: MultipartBody.Part,
        description: MultipartBody.Part? = null,
        focus: MultipartBody.Part? = null,
    ): Response<MediaUploadResult> = Response.error(
        501,
        "".toResponseBody("text/plain".toMediaTypeOrNull()),
    )
}
