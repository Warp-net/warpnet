/* Copyright 2022 Warpdroid Contributors
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

package site.warpnet.warpdroid.viewdata

import android.os.Parcelable
import site.warpnet.warpdroid.entity.Attachment
import kotlinx.parcelize.IgnoredOnParcel
import kotlinx.parcelize.Parcelize

@Parcelize
data class AttachmentViewData(
    val attachment: Attachment,
    val statusId: String?,
    val statusUrl: String?,
    val statusAuthorId: String?,
    val sensitive: Boolean,
    val isRevealed: Boolean
) : Parcelable {

    @IgnoredOnParcel
    val id = attachment.id

    companion object {
        @JvmStatic
        fun list(
            status: TweetViewData.Concrete,
            alwaysShowSensitiveMedia: Boolean = false
        ): List<AttachmentViewData> {
            return status.attachments.map { attachment ->
                AttachmentViewData(
                    attachment = attachment,
                    statusId = status.actionableId,
                    statusUrl = status.actionable.url!!,
                    statusAuthorId = status.actionable.account.id,
                    sensitive = status.actionable.sensitive,
                    isRevealed = alwaysShowSensitiveMedia || !status.actionable.sensitive
                )
            }
        }
    }
}
