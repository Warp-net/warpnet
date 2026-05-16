/* Copyright 2019 kyori19
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

import site.warpnet.warpdroid.json.StringOrBoolean
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass
import java.util.Date

@JsonClass(generateAdapter = true)
data class ScheduledTweet(
    val id: String,
    @Json(name = "scheduled_at") val scheduledAt: Date,
    val params: TweetParams,
    @Json(name = "media_attachments")
    val mediaAttachments: List<Attachment>
)

@JsonClass(generateAdapter = true)
data class TweetParams(
    val text: String,
    // https://github.com/mastodon/mastodon/issues/24645 🙄
    // null on GoToSocial
    @StringOrBoolean val sensitive: Boolean?,
    val visibility: Tweet.Visibility,
    @Json(name = "spoiler_text") val spoilerText: String?,
    @Json(name = "in_reply_to_id") val inReplyToId: String?,
    val language: String,
    // null on GoToSocial when there are no media attachments
    @Json(name = "media_ids") val mediaIds: List<String>?,
)

// minimal class to avoid json parsing errors with servers that don't support scheduling
// https://github.com/tuskyapp/Tusky/issues/4703
@JsonClass(generateAdapter = true)
data class ScheduledTweetReply(
    val id: String,
)
