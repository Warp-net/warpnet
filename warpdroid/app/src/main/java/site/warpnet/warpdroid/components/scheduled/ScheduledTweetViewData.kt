/* Copyright 2025 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.scheduled

import androidx.compose.runtime.Stable
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.ScheduledTweet
import site.warpnet.warpdroid.entity.Tweet
import java.util.Date

@Stable
data class ScheduledTweetViewData(
    val id: String,
    val scheduledAt: Date,
    val text: String,
    val spoilerText: String?,
    val visibility: Tweet.Visibility,
    val sensitive: Boolean,
    val inReplyToId: String?,
    val language: String,
    val attachments: List<Attachment>,
    val spoilerExpanded: Boolean,
    val overflowVisible: Boolean,
    val mediaVisible: Boolean
) {
    val hasLongText = text.length > 750 || text.count { char -> char == '\n' } > 14
}

fun ScheduledTweet.toViewData(
    spoilerExpanded: Boolean,
    overflowVisible: Boolean,
    mediaVisible: Boolean
): ScheduledTweetViewData {
    // mediaAttachments has the full attachment data, params.mediaIds specifies the correct order 🙄
    val attachments: List<Attachment> = params.mediaIds.orEmpty()
        .mapNotNull { mediaId ->
            mediaAttachments.find { attachment -> attachment.id == mediaId }
        }

    return ScheduledTweetViewData(
        id = id,
        scheduledAt = scheduledAt,
        // Warpnet trims texts when posting, but scheduled posts still have their original whitespace
        text = params.text.trim(),
        spoilerText = params.spoilerText,
        visibility = params.visibility,
        sensitive = params.sensitive == true,
        inReplyToId = params.inReplyToId,
        language = params.language,
        attachments = attachments,
        spoilerExpanded = spoilerExpanded,
        overflowVisible = overflowVisible,
        mediaVisible = mediaVisible
    )
}
