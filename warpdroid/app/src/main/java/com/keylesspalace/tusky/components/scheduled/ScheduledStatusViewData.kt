/* Copyright 2025 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.components.scheduled

import androidx.compose.runtime.Stable
import com.keylesspalace.tusky.entity.Attachment
import com.keylesspalace.tusky.entity.NewPoll
import com.keylesspalace.tusky.entity.ScheduledStatus
import com.keylesspalace.tusky.entity.Status
import java.util.Date

@Stable
data class ScheduledStatusViewData(
    val id: String,
    val scheduledAt: Date,
    val text: String,
    val spoilerText: String?,
    val visibility: Status.Visibility,
    val sensitive: Boolean,
    val inReplyToId: String?,
    val language: String,
    val attachments: List<Attachment>,
    val poll: NewPoll?,
    val spoilerExpanded: Boolean,
    val overflowVisible: Boolean,
    val mediaVisible: Boolean
) {
    val hasLongText = text.length > 750 || text.count { char -> char == '\n' } > 14
}

fun ScheduledStatus.toViewData(
    spoilerExpanded: Boolean,
    overflowVisible: Boolean,
    mediaVisible: Boolean
): ScheduledStatusViewData {
    // mediaAttachments has the full attachment data, params.mediaIds specifies the correct order 🙄
    val attachments: List<Attachment> = params.mediaIds.orEmpty()
        .mapNotNull { mediaId ->
            mediaAttachments.find { attachment -> attachment.id == mediaId }
        }

    return ScheduledStatusViewData(
        id = id,
        scheduledAt = scheduledAt,
        // Mastodon trims texts when posting, but scheduled posts still have their original whitespace
        text = params.text.trim(),
        spoilerText = params.spoilerText,
        visibility = params.visibility,
        sensitive = params.sensitive == true,
        inReplyToId = params.inReplyToId,
        language = params.language,
        attachments = attachments,
        poll = params.poll,
        spoilerExpanded = spoilerExpanded,
        overflowVisible = overflowVisible,
        mediaVisible = mediaVisible
    )
}
