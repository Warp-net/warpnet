/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.network

import okio.Buffer
import retrofit2.Response
import site.warpnet.transport.WarpnetLimits
import site.warpnet.warpdroid.entity.MediaUploadResult
import site.warpnet.warpdroid.warpnet.WarpnetRepository
import javax.inject.Inject
import javax.inject.Singleton
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.MultipartBody
import okhttp3.ResponseBody.Companion.toResponseBody

object MediaKind {
    const val IMAGE_PREFIX = "img:"
    const val VIDEO_PREFIX = "vid:"

    fun tagImage(key: String): String = IMAGE_PREFIX + key
    fun tagVideo(key: String): String = VIDEO_PREFIX + key

    fun isVideo(id: String): Boolean = id.startsWith(VIDEO_PREFIX)

    fun untag(id: String): String = id
        .removePrefix(IMAGE_PREFIX)
        .removePrefix(VIDEO_PREFIX)
}

@Singleton
class MediaUploadApi @Inject constructor(
    private val warpnet: WarpnetRepository,
) {
    suspend fun uploadMedia(
        file: MultipartBody.Part,
        description: MultipartBody.Part? = null,
        focus: MultipartBody.Part? = null,
    ): Response<MediaUploadResult> {
        val body = file.body
        val mimeType = body.contentType()?.toString().orEmpty()

        val bytes = try {
            Buffer().also { body.writeTo(it) }.readByteArray()
        } catch (e: Exception) {
            return failure(500, "could not read the attachment: ${e.message}")
        }
        if (bytes.isEmpty()) {
            return failure(400, "the attachment is empty")
        }

        val isVideo = mimeType.startsWith("video/", ignoreCase = true)
        if (isVideo) {
            if (mimeType.lowercase() !in WarpnetLimits.ACCEPTED_VIDEO_MIME_TYPES) {
                return failure(415, "only MP4, QuickTime and M4V videos are accepted")
            }
            if (bytes.size > WarpnetLimits.MAX_VIDEO_BYTES) {
                return failure(413, "the video is larger than 36 MiB")
            }
        } else if (!mimeType.startsWith("image/", ignoreCase = true)) {
            return failure(415, "only images and videos can be attached")
        }

        val base64 = android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP)

        return try {
            if (isVideo) {
                val key = warpnet.uploadVideo("data:$mimeType;base64,$base64")
                if (key.isBlank()) {
                    failure(502, "the node stored no video key")
                } else {
                    Response.success(MediaUploadResult(id = MediaKind.tagVideo(key)))
                }
            } else {
                val key = warpnet.uploadImage("$mimeType,$base64")
                if (key.isBlank()) {
                    failure(502, "the node stored no image key")
                } else {
                    Response.success(MediaUploadResult(id = MediaKind.tagImage(key)))
                }
            }
        } catch (e: Exception) {
            failure(502, e.message ?: "upload failed")
        }
    }

    private fun failure(code: Int, message: String): Response<MediaUploadResult> = Response.error(
        code,
        message.toResponseBody("text/plain".toMediaTypeOrNull()),
    )
}
