/* Copyright 2024 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.notifications

import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.util.toViewData
import site.warpnet.warpdroid.viewdata.NotificationViewData

// The Room-backed entity mappers that used to live here were deleted alongside
// AppDatabase. Only the network-only Notification -> NotificationViewData
// converter is still needed, by NotificationRequestDetailsRemoteMediator.
fun Notification.toViewData(
    isShowingContent: Boolean,
    isExpanded: Boolean,
    isCollapsed: Boolean,
    filterKind: Filter.Kind,
    isQuoteShowingContent: Boolean,
    isQuoteExpanded: Boolean,
    isQuoteCollapsed: Boolean,
    isQuoteShown: Boolean
): NotificationViewData.Concrete = NotificationViewData.Concrete(
    id = id,
    type = type,
    account = account,
    statusViewData = status?.toViewData(
        isShowingContent = isShowingContent,
        isExpanded = isExpanded,
        isCollapsed = isCollapsed,
        filterKind = filterKind,
        filterActive = true,
        isQuoteShowingContent = isQuoteShowingContent,
        isQuoteExpanded = isQuoteExpanded,
        isQuoteCollapsed = isQuoteCollapsed,
        isQuoteShown = isQuoteShown
    ),
    moderationWarning = moderationWarning,
    event = event,
    emoji = emoji,
    emojiUrl = emojiUrl,
)
