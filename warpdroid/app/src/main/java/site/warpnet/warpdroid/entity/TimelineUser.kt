/* Copyright 2017 Andrew Dawson
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.entity

import androidx.compose.runtime.Immutable
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Same as [User], but only with the attributes required in timelines.
 * Prefer this class over [User] because it uses way less memory & deserializes faster from json.
 */
@Immutable
@JsonClass(generateAdapter = true)
data class TimelineUser(
    val id: String,
    @Json(name = "username") val localUsername: String,
    @Json(name = "acct") val username: String,
    // should never be null per Api definition, but some servers break the contract
    @Json(name = "display_name") val displayName: String? = null,
    val url: String,
    val avatar: String,
    @Json(name = "avatar_static") val staticAvatar: String,
    val note: String,
    val bot: Boolean = false,
    // optional for backward compatibility
    val emojis: List<Emoji> = emptyList()
) {

    val name: String
        get() = if (displayName.isNullOrEmpty()) {
            localUsername
        } else {
            displayName
        }
}
