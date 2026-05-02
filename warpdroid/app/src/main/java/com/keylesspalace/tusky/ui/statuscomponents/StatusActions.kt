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

package com.keylesspalace.tusky.ui.statuscomponents

import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.util.reply
import com.keylesspalace.tusky.util.showFavs
import com.keylesspalace.tusky.util.showReblogs
import com.keylesspalace.tusky.viewdata.StatusViewData

@Composable
fun statusActions(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener
): List<CustomAccessibilityAction> = buildList {
    val status = statusViewData.actionable

    val activeAccount = LocalAccount.current
    val context = LocalContext.current

    if (status.spoilerText.isNotEmpty()) {
        addAction(
            label = if (statusViewData.isExpanded) {
                stringResource(R.string.post_content_warning_show_less)
            } else {
                stringResource(R.string.post_content_warning_show_more)
            },
            action = {
                listener.onExpandedChange(statusViewData, !statusViewData.isExpanded)
            }
        )
    }

    addAction(
        label = stringResource(R.string.action_reply),
        action = {
            activeAccount?.let {
                context.reply(statusViewData, it)
            }
        }
    )

    if (status.isRebloggingAllowed) {
        addAction(
            label = if (status.reblogged) {
                stringResource(R.string.action_unreblog)
            } else {
                stringResource(R.string.action_reblog)
            },
            action = {
                listener.onReblog(statusViewData, !status.reblogged, null, null)
            }
        )
    }

    addAction(
        label = if (status.favourited) {
            stringResource(R.string.action_unfavourite)
        } else {
            stringResource(R.string.action_favourite)
        },
        action = {
            listener.onFavourite(statusViewData, !status.favourited, null)
        }
    )

    addAction(
        label = if (status.bookmarked) {
            stringResource(R.string.action_unbookmark)
        } else {
            stringResource(R.string.action_bookmark)
        },
        action = {
            listener.onBookmark(statusViewData, !status.bookmarked)
        }
    )

    if (status.attachments.isNotEmpty() && LocalAccount.current?.mediaPreviewEnabled == true) {
        addAction(
            label = if (statusViewData.isShowingContent) {
                stringResource(R.string.action_hide_media)
            } else {
                stringResource(R.string.action_reveal_media)
            },
            action = {
                listener.onContentHiddenChange(statusViewData, !statusViewData.isShowingContent)
            }
        )
    }

    status.attachments.indices.forEach { index ->
        addAction(
            label = stringResource(R.string.action_open_media_n, index + 1),
            action = {
                listener.onViewMedia(statusViewData, index)
            }
        )
    }

    addAction(
        label = stringResource(R.string.action_view_profile),
        action = {
            listener.onViewAccount(status.account.id)
        }
    )

    if (status.reblog?.account != null) {
        addAction(
            label = stringResource(R.string.action_open_reblogger),
            action = {
                listener.onViewAccount(status.reblog.account.id)
            }
        )
    }

    if (status.reblogsCount > 0) {
        addAction(
            label = stringResource(R.string.action_open_reblogged_by),
            action = {
                context.showReblogs(statusViewData)
            }
        )
    }

    if (status.favouritesCount > 0) {
        addAction(
            label = stringResource(R.string.action_open_faved_by),
            action = {
                context.showFavs(statusViewData)
            }
        )
    }

    if (status.account.id == activeAccount?.accountId) {
        addAction(
            label = stringResource(R.string.action_edit),
            action = {
                listener.onEdit(statusViewData)
            }
        )
        addAction(
            label = stringResource(R.string.action_delete),
            action = {
                listener.onDelete(statusViewData)
            }
        )
        addAction(
            label = stringResource(R.string.action_delete_and_redraft),
            action = {
                listener.onRedraft(statusViewData)
            }
        )
    } else {
        status.quote?.quotedStatus?.let { quotedStatus ->
            if (quotedStatus.account.id == activeAccount?.accountId) {
                addAction(
                    label = stringResource(R.string.action_remove_quote),
                    action = {
                        listener.removeQuote(statusViewData)
                    }
                )
            }
        }
    }
}

private fun MutableList<CustomAccessibilityAction>.addAction(
    label: String,
    action: () -> Unit
) {
    add(
        CustomAccessibilityAction(
            label = label,
            action = {
                action()
                true
            }
        )
    )
}
