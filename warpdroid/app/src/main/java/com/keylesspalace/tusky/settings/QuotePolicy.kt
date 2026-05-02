package com.keylesspalace.tusky.settings

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = false)
enum class QuotePolicy(val text: String) {
    @Json(name = "public")
    PUBLIC("public"),

    @Json(name = "followers")
    FOLLOWERS("followers"),

    @Json(name = "nobody")
    NOBODY("nobody");

    companion object {
        fun forValue(text: String) = entries.firstOrNull { it.text == text }
    }
}
