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

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import at.connyduck.sparkbutton.compose.SparkButton
import at.connyduck.sparkbutton.compose.rememberSparkButtonState
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.WarpdroidPreviewTheme
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.fake.fakeTweetViewData
import site.warpnet.warpdroid.ui.tweetcomponents.fake.noopListener
import site.warpnet.warpdroid.ui.warpdroidBlueDark
import site.warpnet.warpdroid.ui.warpdroidBlueLight
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.ui.warpdroidGreenDark
import site.warpnet.warpdroid.ui.warpdroidGreenLight
import site.warpnet.warpdroid.ui.warpdroidOrange
import site.warpnet.warpdroid.ui.warpdroidOrangeLight
import site.warpnet.warpdroid.util.formatNumber
import site.warpnet.warpdroid.viewdata.TweetViewData

@Composable
fun TweetButtons(
    statusViewData: TweetViewData.Concrete,
    listener: TweetActionListener,
    accounts: List<AccountEntity>,
    modifier: Modifier = Modifier,
    showStats: Boolean = !statusViewData.isDetailed && LocalPreferences.current.showStatsInline
) {
    val status = statusViewData.actionable

    val description = buildString {
        if (status.retweeted) {
            append(stringResource(R.string.description_post_retweeted))
            append(", ")
        }
        if (status.liked) {
            append(stringResource(R.string.description_post_liked))
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
            if (status.retweetsCount > 0) {
                append(pluralStringResource(R.plurals.retweets, status.retweetsCount, status.retweetsCount))
                append(", ")
            }
            if (status.likesCount > 0) {
                append(pluralStringResource(R.plurals.favs, status.likesCount, status.likesCount))
                append(", ")
            }
        }
    }

    // Plain Row instead of ConstraintLayout with a horizontal chain: each
    // row of TweetButtons gets measured once per LazyList item, and the
    // chain solver was a measurable cost on first frame of Profile / Home.
    // LTR/RTL is handled natively by Row.
    Row(
        modifier = modifier
            .clearAndSetSemantics {
                contentDescription = description
            },
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // TODO: properly connect these to the confirmation bottom sheet once it is in Compose
        var retweeted by remember(status.retweeted) { mutableStateOf(status.retweeted) }
        var liked by remember(status.liked) { mutableStateOf(status.liked) }
        var bookmarked by remember(status.bookmarked) { mutableStateOf(status.bookmarked) }

        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(
                onClick = {
                    listener.onReply(statusViewData)
                },
            ) {
                Icon(
                    painter = if (status.isReply) {
                        painterResource(R.drawable.ic_reply_all_24dp)
                    } else {
                        painterResource(R.drawable.ic_reply_24dp)
                    },
                    tint = warpdroidColors.tertiaryTextColor,
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
                        stringResource(R.string.tweet_count_one_plus)
                    },
                    color = warpdroidColors.tertiaryTextColor,
                    style = LocalPreferences.current.statusTextStyles.medium,
                )
            }
        }

        // Warpnet only emits Tweet.Visibility.PUBLIC (WarpnetMapper.toTweet
        // hardcodes it), so the DIRECT / PRIVATE Tusky branches that used
        // to render a mail or lock icon in place of the retweet button
        // never fired here. Inlining the public path directly.
        //
        // Vue desktop UX: tapping retweet on a not-yet-retweeted post
        // opens a small menu with "Retweet" / "Quote"; tapping on an
        // already-retweeted post untoggles directly without a menu.
        // SparkButton's reveal animation is kept for the plain retweet
        // path because that's where the celebration fires; the quote
        // path opens a new screen so the animation would never play.
        //
        // The DropdownMenu is wrapped in an inner Box with .wrapContentSize
        // so Popup's anchor-positioning code reads a tight, stable
        // bounding box right under the icon. Without that the menu
        // appeared at apparently-random screen positions on different
        // tweet rows.
        val retweetSparkButtonState = rememberSparkButtonState()
        var showRetweetMenu by remember { mutableStateOf(false) }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(modifier = Modifier.wrapContentSize()) {
                SparkButton(
                    animateOnClick = false,
                    onClick = {
                        if (retweeted) {
                            listener.onRetweet(statusViewData, false, null, state = retweetSparkButtonState)
                        } else {
                            showRetweetMenu = true
                        }
                    },
                    state = retweetSparkButtonState,
                    primaryColor = warpdroidBlueDark,
                    secondaryColor = warpdroidBlueLight,
                ) {
                    if (retweeted) {
                        Icon(
                            painter = painterResource(R.drawable.ic_repeat_active_24dp),
                            tint = colorScheme.primary,
                            contentDescription = null,
                            modifier = Modifier.size(24.dp)
                        )
                    } else {
                        Icon(
                            painter = painterResource(R.drawable.ic_repeat_24dp),
                            tint = warpdroidColors.tertiaryTextColor,
                            contentDescription = null,
                            modifier = Modifier.size(24.dp)
                        )
                    }
                }
                DropdownMenu(
                    expanded = showRetweetMenu,
                    onDismissRequest = { showRetweetMenu = false },
                ) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.action_retweet)) },
                        onClick = {
                            showRetweetMenu = false
                            listener.onRetweet(statusViewData, true, null, state = retweetSparkButtonState)
                        },
                    )
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.action_quote)) },
                        onClick = {
                            showRetweetMenu = false
                            listener.onQuote(statusViewData)
                        },
                    )
                }
            }
            if (showStats) {
                Text(
                    text = formatNumber(status.retweetsCount.toLong(), 1000),
                    color = if (retweeted) {
                        colorScheme.primary
                    } else {
                        warpdroidColors.tertiaryTextColor
                    },
                    style = LocalPreferences.current.statusTextStyles.medium,
                )
            }
        }

        val sparkButtonState = rememberSparkButtonState()
        Row(verticalAlignment = Alignment.CenterVertically) {
            SparkButton(
                animateOnClick = false,
                onClick = {
                    listener.onLike(statusViewData, !liked, state = sparkButtonState)
                },
                state = sparkButtonState,
                primaryColor = warpdroidOrange,
                secondaryColor = warpdroidOrangeLight,
            ) {
                if (liked) {
                    Icon(
                        painter = painterResource(R.drawable.ic_star_24dp_filled),
                        tint = warpdroidColors.likeButtonActiveColor,
                        contentDescription = null,
                        modifier = Modifier.size(24.dp)
                    )
                } else {
                    Icon(
                        painter = painterResource(R.drawable.ic_star_24dp),
                        tint = warpdroidColors.tertiaryTextColor,
                        contentDescription = null,
                        modifier = Modifier.size(24.dp)
                    )
                }
            }
            if (showStats) {
                Text(
                    text = formatNumber(status.likesCount.toLong(), 1000),
                    color = if (liked) {
                        warpdroidColors.likeButtonActiveColor
                    } else {
                        warpdroidColors.tertiaryTextColor
                    },
                    style = LocalPreferences.current.statusTextStyles.medium,
                )
            }
        }

        SparkButton(
            animateOnClick = !bookmarked,
            onClick = {
                bookmarked = !bookmarked
                listener.onBookmark(statusViewData, bookmarked)
            },
            primaryColor = warpdroidGreenDark,
            secondaryColor = warpdroidGreenLight,
        ) {
            if (bookmarked) {
                Icon(
                    painter = painterResource(R.drawable.ic_bookmark_24dp_filled),
                    tint = warpdroidColors.bookmarkButtonActiveColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            } else {
                Icon(
                    painter = painterResource(R.drawable.ic_bookmark_24dp),
                    tint = warpdroidColors.tertiaryTextColor,
                    contentDescription = null,
                    modifier = Modifier.size(24.dp)
                )
            }
        }

        var moreVisible by remember { mutableStateOf(false) }
        Box {
            IconButton(
                onClick = {
                    moreVisible = !moreVisible
                }
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_more_horiz_24dp),
                    tint = warpdroidColors.tertiaryTextColor,
                    contentDescription = null
                )
            }
            TweetMoreMenu(
                viewData = statusViewData,
                expanded = moreVisible,
                onDismissRequest = {
                    moreVisible = !moreVisible
                },
                accounts = accounts,
                listener = listener
            )
        }
    }
}

@PreviewLightDark
@Composable
fun TweetButtonsPreview() {
    WarpdroidPreviewTheme {
        TweetButtons(
            statusViewData = fakeTweetViewData(),
            listener = noopListener,
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
fun TweetButtonsWithStatsPreview() {
    WarpdroidPreviewTheme {
        TweetButtons(
            statusViewData = fakeTweetViewData(),
            listener = noopListener,
            accounts = emptyList(),
            modifier = Modifier
                .width(320.dp)
                .background(colorScheme.background),
            showStats = true
        )
    }
}
