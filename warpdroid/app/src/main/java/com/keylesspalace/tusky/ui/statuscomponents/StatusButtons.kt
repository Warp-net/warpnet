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

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.constraintlayout.compose.ChainStyle
import androidx.constraintlayout.compose.ConstraintLayout
import at.connyduck.sparkbutton.compose.SparkButton
import at.connyduck.sparkbutton.compose.rememberSparkButtonState
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeStatusViewData
import com.keylesspalace.tusky.ui.statuscomponents.fake.noopListener
import com.keylesspalace.tusky.ui.tuskyBlueDark
import com.keylesspalace.tusky.ui.tuskyBlueLight
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.ui.tuskyGreenDark
import com.keylesspalace.tusky.ui.tuskyGreenLight
import com.keylesspalace.tusky.ui.tuskyOrange
import com.keylesspalace.tusky.ui.tuskyOrangeLight
import com.keylesspalace.tusky.util.formatNumber
import com.keylesspalace.tusky.viewdata.StatusViewData

@Composable
fun StatusButtons(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener,
    translationEnabled: Boolean,
    accounts: List<AccountEntity>,
    modifier: Modifier = Modifier,
    showStats: Boolean = !statusViewData.isDetailed && LocalPreferences.current.showStatsInline
) {
    val status = statusViewData.actionable

    val description = buildString {
        if (status.reblogged) {
            append(stringResource(R.string.description_post_reblogged))
            append(", ")
        }
        if (status.favourited) {
            append(stringResource(R.string.description_post_favourited))
            append(", ")
        }
        if (status.bookmarked) {
            append(stringResource(R.string.description_post_bookmarked))
            append(", ")
        }
        if (showStats) {
            if (status.repliesCount > 0) {
                append(pluralStringResource(R.plurals.replies, status.repliesCount, status.repliesCount))
                append(", ")
            }
            if (status.reblogsCount > 0) {
                append(pluralStringResource(R.plurals.reblogs, status.reblogsCount, status.reblogsCount))
                append(", ")
            }
            if (status.favouritesCount > 0) {
                append(pluralStringResource(R.plurals.favs, status.favouritesCount, status.favouritesCount))
                append(", ")
            }
        }
    }

    ConstraintLayout(
        modifier = modifier
            .clearAndSetSemantics {
                contentDescription = description
            },
    ) {
        // TODO: properly connect these to the confirmation bottom sheet once it is in Compose
        var reblogged by remember(status.reblogged) { mutableStateOf(status.reblogged) }
        var favourited by remember(status.favourited) { mutableStateOf(status.favourited) }
        var bookmarked by remember(status.bookmarked) { mutableStateOf(status.bookmarked) }

        val (replyButton, replyCount, reblogButton, reblogCount, favButton, favCount, bookmarkButton, moreButton) = createRefs()

        // work around for https://issuetracker.google.com/issues/455056601
        if (LocalLayoutDirection.current == LayoutDirection.Ltr) {
            createHorizontalChain(replyButton, reblogButton, favButton, bookmarkButton, moreButton, chainStyle = ChainStyle.SpreadInside)
        } else {
            createHorizontalChain(moreButton, bookmarkButton, favButton, reblogButton, replyButton, chainStyle = ChainStyle.SpreadInside)
        }
        IconButton(
            onClick = {
                listener.onReply(statusViewData)
            },
            modifier = Modifier.constrainAs(replyButton) {
                start.linkTo(parent.start)
                end.linkTo(reblogButton.start)
                centerVerticallyTo(parent)
            }
        ) {
            Icon(
                painter = if (status.isReply) {
                    painterResource(R.drawable.ic_reply_all_24dp)
                } else {
                    painterResource(R.drawable.ic_reply_24dp)
                },
                tint = tuskyColors.tertiaryTextColor,
                contentDescription = null
            )
        }
        if (!statusViewData.isDetailed) {
            Text(
                text = if (showStats) {
                    formatNumber(status.repliesCount.toLong(), 1000)
                } else if (status.repliesCount == 0) {
                    "0"
                } else if (status.repliesCount == 1) {
                    "1"
                } else {
                    stringResource(R.string.status_count_one_plus)
                },
                color = tuskyColors.tertiaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                modifier = Modifier.constrainAs(replyCount) {
                    start.linkTo(replyButton.end)
                    centerVerticallyTo(parent)
                }
            )
        }

        if (status.visibility == Status.Visibility.DIRECT) {
            Icon(
                painter = painterResource(R.drawable.ic_mail_24dp),
                tint = tuskyColors.disabledTextColor,
                contentDescription = null,
                modifier = Modifier.size(24.dp)
                    .constrainAs(reblogButton) {
                        start.linkTo(replyButton.end)
                        end.linkTo(favButton.start)
                        centerVerticallyTo(parent)
                    }
            )
        } else if (status.visibility == Status.Visibility.PRIVATE) {
            Icon(
                painter = if (reblogged) {
                    painterResource(R.drawable.ic_lock_24dp_filled)
                } else {
                    painterResource(R.drawable.ic_lock_24dp)
                },
                tint = if (reblogged) {
                    colorScheme.primary
                } else {
                    tuskyColors.disabledTextColor
                },
                contentDescription = null,
                modifier = Modifier.size(24.dp)
                    .constrainAs(reblogButton) {
                        start.linkTo(replyButton.end)
                        end.linkTo(favButton.start)
                        centerVerticallyTo(parent)
                    }
            )
        } else {
            val sparkButtonState = rememberSparkButtonState()
            SparkButton(
                animateOnClick = false,
                onClick = {
                    listener.onReblog(statusViewData, !reblogged, null, state = sparkButtonState)
                },
                state = sparkButtonState,
                primaryColor = tuskyBlueDark,
                secondaryColor = tuskyBlueLight,
                modifier = Modifier
                    .constrainAs(reblogButton) {
                        start.linkTo(replyButton.end)
                        end.linkTo(favButton.start)
                        centerVerticallyTo(parent)
                    }
            ) {
                if (reblogged) {
                    Icon(
                        painter = painterResource(R.drawable.ic_repeat_active_24dp),
                        tint = colorScheme.primary,
                        contentDescription = null,
                        modifier = Modifier.size(24.dp)
                    )
                } else {
                    Icon(
                        painter = painterResource(R.drawable.ic_repeat_24dp),
                        tint = tuskyColors.tertiaryTextColor,
                        contentDescription = null,
                        modifier = Modifier.size(24.dp)
                    )
                }
            }

            if (showStats) {
                Text(
                    text = formatNumber(status.reblogsCount.toLong(), 1000),
                    color = if (reblogged) {
                        colorScheme.primary
                    } else {
                        tuskyColors.tertiaryTextColor
                    },
                    style = LocalPreferences.current.statusTextStyles.medium,
                    modifier = Modifier
                        .constrainAs(reblogCount) {
                            start.linkTo(reblogButton.end, margin = 4.dp)
                            centerVerticallyTo(parent)
                        }
                )
            }
        }

        val sparkButtonState = rememberSparkButtonState()
        SparkButton(
            animateOnClick = false,
            onClick = {
                listener.onFavourite(statusViewData, !favourited, state = sparkButtonState)
            },
            state = sparkButtonState,
            primaryColor = tuskyOrange,
            secondaryColor = tuskyOrangeLight,
            modifier = Modifier.constrainAs(favButton) {
                start.linkTo(reblogButton.end)
                end.linkTo(bookmarkButton.start)
                centerVerticallyTo(parent)
            }
        ) {
            if (favourited) {
                Icon(
                    painter = painterResource(R.drawable.ic_star_24dp_filled),
                    tint = tuskyColors.favoriteButtonActiveColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            } else {
                Icon(
                    painter = painterResource(R.drawable.ic_star_24dp),
                    tint = tuskyColors.tertiaryTextColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            }
        }
        if (showStats) {
            Text(
                text = formatNumber(status.favouritesCount.toLong(), 1000),
                color = if (favourited) {
                    tuskyColors.favoriteButtonActiveColor
                } else {
                    tuskyColors.tertiaryTextColor
                },
                style = LocalPreferences.current.statusTextStyles.medium,
                modifier = Modifier
                    .constrainAs(favCount) {
                        start.linkTo(favButton.end, margin = 4.dp)
                        centerVerticallyTo(parent)
                    }
            )
        }

        SparkButton(
            animateOnClick = !bookmarked,
            onClick = {
                bookmarked = !bookmarked
                listener.onBookmark(statusViewData, bookmarked)
            },
            primaryColor = tuskyGreenDark,
            secondaryColor = tuskyGreenLight,
            modifier = Modifier.constrainAs(bookmarkButton) {
                start.linkTo(favButton.end)
                end.linkTo(moreButton.start)
                centerVerticallyTo(parent)
            }
        ) {
            if (bookmarked) {
                Icon(
                    painter = painterResource(R.drawable.ic_bookmark_24dp_filled),
                    tint = tuskyColors.bookmarkButtonActiveColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            } else {
                Icon(
                    painter = painterResource(R.drawable.ic_bookmark_24dp),
                    tint = tuskyColors.tertiaryTextColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            }
        }

        var moreVisible by remember { mutableStateOf(false) }

        Box(
            modifier = Modifier.constrainAs(moreButton) {
                start.linkTo(bookmarkButton.end)
                end.linkTo(parent.end)
                centerVerticallyTo(parent)
            }
        ) {
            IconButton(
                onClick = {
                    moreVisible = !moreVisible
                }
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_more_horiz_24dp),
                    tint = tuskyColors.tertiaryTextColor,
                    contentDescription = null
                )
            }
            StatusMoreMenu(
                viewData = statusViewData,
                expanded = moreVisible,
                onDismissRequest = {
                    moreVisible = !moreVisible
                },
                translationEnabled = translationEnabled,
                accounts = accounts,
                listener = listener
            )
        }
    }
}

@PreviewLightDark
@Composable
fun StatusButtonsPreview() {
    TuskyPreviewTheme {
        StatusButtons(
            statusViewData = fakeStatusViewData(),
            listener = noopListener,
            translationEnabled = false,
            accounts = emptyList(),
            modifier = Modifier
                .width(320.dp)
                .background(colorScheme.background),
            showStats = false
        )
    }
}

@PreviewLightDark
@Composable
fun StatusButtonsWithStatsPreview() {
    TuskyPreviewTheme {
        StatusButtons(
            statusViewData = fakeStatusViewData(),
            listener = noopListener,
            translationEnabled = false,
            accounts = emptyList(),
            modifier = Modifier
                .width(320.dp)
                .background(colorScheme.background),
            showStats = true
        )
    }
}
