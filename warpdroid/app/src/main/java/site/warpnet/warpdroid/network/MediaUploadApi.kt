/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Bridges [site.warpnet.warpdroid.components.compose.MediaUploader]'s
 * multipart-shaped call onto Warpnet's blob routes. The node takes media as a
 * base64 string in a JSON body — `/private/post/image` for images (four slots
 * per call, one used here) and `/private/post/video` for video — and answers
 * with a content-addressed key. That key is the attachment id everywhere
 * downstream.
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

/**
 * Warpnet keys carry no type marker, but a tweet splits its attachments into
 * `image_keys` and a single `video_key`. The upload is the only place that
 * still knows which kind it handled, so it tags the id it hands back and
 * [WarpnetApi.createStatus] reads the tag off again.
 */
object MediaKind {
    const val IMAGE_PREFIX = "img:"
    const val VIDEO_PREFIX = "vid:"

    fun tagImage(key: String): String = IMAGE_PREFIX + key
    fun tagVideo(key: String): String = VIDEO_PREFIX + key

    fun isVideo(id: String): Boolean = id.startsWith(VIDEO_PREFIX)

    /** Strip whichever tag [id] carries; an untagged id is returned as-is. */
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

        // The wire is base64 inside JSON, so the whole file has to be
        // resident: writing the part into a buffer also drives the progress
        // callback the uploader attached to it. Peak cost is a few times the
        // file size, which is why the size checks below run before the
        // encode and why the video cap is the node's, not Mastodon's.
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
                // The video route reads a data URL; the image route reads a
                // bare "<mime>,<base64>" pair. Both shapes come straight from
                // core/handler, which parses them differently.
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
