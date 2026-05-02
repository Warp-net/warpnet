package com.keylesspalace.tusky.entity

import androidx.compose.runtime.Stable
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@Stable
@JsonClass(generateAdapter = true)
data class MediaTranslation(
    val id: String,
    val description: String,
)

/**
 * Represents the result of machine translating some status content.
 *
 * See [doc](https://docs.joinmastodon.org/entities/Translation/).
 */
@Stable
@JsonClass(generateAdapter = true)
data class Translation(
    val content: String,
    @Json(name = "spoiler_text")
    val spoilerText: String? = null,
    val poll: TranslatedPoll? = null,
    @Json(name = "media_attachments")
    val mediaAttachments: List<MediaTranslation> = emptyList(),
    @Json(name = "detected_source_language")
    val detectedSourceLanguage: String,
    val provider: String,
)

@Stable
@JsonClass(generateAdapter = true)
data class TranslatedPoll(
    val options: List<TranslatedPollOption>
)

@Stable
@JsonClass(generateAdapter = true)
data class TranslatedPollOption(
    val title: String
)
