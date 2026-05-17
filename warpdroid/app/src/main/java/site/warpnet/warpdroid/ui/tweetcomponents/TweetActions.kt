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

package site.warpnet.warpdroid.ui.tweetcomponents

import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.preferences.LocalAccount
import site.warpnet.warpdroid.util.reply
import site.warpnet.warpdroid.util.showFavs
import site.warpnet.warpdroid.util.showRetweets
import site.warpnet.warpdroid.viewdata.TweetViewData

@Composable
fun statusActions(
    statusViewData: TweetViewData.Concrete,
    listener: TweetActionListener
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

    if (status.isRetweetgingAllowed) {
        addAction(
            label = if (status.retweeted) {
                stringResource(R.string.action_unretweet)
            } else {
                stringResource(R.string.action_retweet)
            },
            action = {
                listener.onRetweet(statusViewData, !status.retweeted, null, null)
            }
        )
    }

    addAction(
        label = if (status.liked) {
            stringResource(R.string.action_unlike)
        } else {
            stringResource(R.string.action_like)
        },
        action = {
            listener.onLike(statusViewData, !status.liked, null)
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

    if (status.retweet?.account != null) {
        addAction(
            label = stringResource(R.string.action_open_retweeter),
            action = {
                listener.onViewAccount(status.retweet.account.id)
            }
        )
    }

    if (status.retweetsCount > 0) {
        addAction(
            label = stringResource(R.string.action_open_retweeted_by),
            action = {
                context.showRetweets(statusViewData)
            }
        )
    }

    if (status.likesCount > 0) {
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
