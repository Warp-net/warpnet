/* Copyright 2023 Warpdroid Contributors
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

import androidx.compose.runtime.Immutable
import site.warpnet.warpdroid.entity.AccountWarning
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.RelationshipSeveranceEvent
import site.warpnet.warpdroid.entity.Report
import site.warpnet.warpdroid.entity.TimelineAccount

@Immutable
sealed class NotificationViewData {

    abstract val id: String

    abstract fun asStatusOrNull(): TweetViewData.Concrete?
    abstract fun asPlaceholderOrNull(): LoadMore?

    @Immutable
    data class Concrete(
        override val id: String,
        val type: Notification.Type,
        val account: TimelineAccount,
        val statusViewData: TweetViewData.Concrete?,
        val report: Report?,
        val event: RelationshipSeveranceEvent?,
        val moderationWarning: AccountWarning?,
        val emoji: String?,
        val emojiUrl: String?,
    ) : NotificationViewData() {
        override fun asStatusOrNull() = statusViewData

        override fun asPlaceholderOrNull() = null
    }

    @Immutable
    data class LoadMore(
        override val id: String,
        override val isLoading: Boolean
    ) : NotificationViewData(), LoadMoreViewData {
        override fun asStatusOrNull() = null

        override fun asPlaceholderOrNull() = this
    }
}
