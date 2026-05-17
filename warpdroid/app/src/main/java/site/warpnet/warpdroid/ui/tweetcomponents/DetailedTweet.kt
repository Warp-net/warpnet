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

import androidx.annotation.DrawableRes
import androidx.annotation.PluralsRes
import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withLink
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.WarpdroidPreviewTheme
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.fake.fakeTweetViewData
import site.warpnet.warpdroid.ui.tweetcomponents.fake.noopListener
import site.warpnet.warpdroid.ui.tweetcomponents.text.emojify
import site.warpnet.warpdroid.ui.tweetcomponents.text.linkStyles
import site.warpnet.warpdroid.ui.tweetcomponents.text.toInlineContent
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.showFavs
import site.warpnet.warpdroid.util.showQuotes
import site.warpnet.warpdroid.util.showRetweets
import site.warpnet.warpdroid.viewdata.TweetViewData
import java.text.DateFormat
import java.text.NumberFormat

@Composable
fun DetailedTweet(
    statusViewData: TweetViewData.Concrete,
    listener: TweetActionListener,
    accounts: List<AccountEntity>,
    modifier: Modifier = Modifier
) {
    val status = statusViewData.actionable

    val actions = statusActions(
        statusViewData = statusViewData,
        listener = listener
    )

    Column(
        modifier
            .clickable {
                listener.onViewThread(statusViewData)
            }
            .semantics(mergeDescendants = true) {
                customActions = actions
            }
    ) {
        SelectionContainer {
            Column(
                modifier = Modifier.padding(start = 14.dp, top = 14.dp, end = 14.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Avatar(
                        url = status.account.avatar,
                        staticUrl = status.account.staticAvatar,
                        isBot = status.account.bot,
                        retweetedAvatarUrl = statusViewData.retweetedAvatar,
                        staticRetweetedAvatarUrl = statusViewData.staticRetweetedAvatar,
                        onOpenProfile = {
                            listener.onViewAccount(status.account.id)
                        },
                        modifier = Modifier.clearAndSetSemantics {
                            hideFromAccessibility()
                        }
                    )

                    Spacer(modifier = Modifier.width(16.dp))

                    Column(
                        modifier = Modifier
                            .clickable {
                                listener.onViewAccount(status.account.id)
                            }
                    ) {
                        val username = stringResource(R.string.post_username_format, status.account.username)

                        Text(
                            text = status.account.name.emojify(status.account.emojis),
                            fontWeight = FontWeight.Bold,
                            color = warpdroidColors.primaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            inlineContent = status.account.emojis.toInlineContent()
                        )
                        Text(
                            text = username,
                            color = warpdroidColors.secondaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
                TweetContent(
                    statusViewData = statusViewData,
                    listener = listener
                )

                DetailedMetadata(
                    statusViewData = statusViewData,
                    listener = listener,
                    modifier = Modifier.padding(top = 8.dp)
                )
                if (!LocalPreferences.current.wellbeing.hideQuantitativeStatsOnPosts) {
                    HorizontalDivider(
                        modifier = Modifier.padding(top = 6.dp)
                    )
                    DetailedStatistics(
                        statusViewData = statusViewData,
                        modifier = Modifier.padding(top = 6.dp)
                    )
                }
            }
        }

        TweetButtons(
            statusViewData = statusViewData,
            listener = listener,
            accounts = accounts,
            showStats = false,
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 8.dp, end = 8.dp, top = 4.dp, bottom = 2.dp)
        )
        HorizontalDivider()
    }
}

private val dateFormat = DateFormat.getDateTimeInstance(DateFormat.DEFAULT, DateFormat.SHORT)

@Composable
private fun DetailedMetadata(
    statusViewData: TweetViewData.Concrete,
    listener: TweetActionListener,
    modifier: Modifier = Modifier
) {
    val status = statusViewData.actionable
    val metadataJoiner = stringResource(R.string.metadata_joiner)
    val metadata = buildAnnotatedString {
        getVisibilityText(status.visibility)?.let { visibilityText ->
            appendInlineContent("visibility", stringResource(visibilityText))
            append(" ")
        }

        append(dateFormat.format(status.createdAt))

        val editedAt = status.editedAt

        if (editedAt != null) {
            // Warpnet has no revision-history endpoint — render the edited
            // timestamp as plain text instead of a link into a dead viewer.
            append(metadataJoiner)
            append(stringResource(R.string.post_edited, dateFormat.format(editedAt)))
        }

        if (status.language != null) {
            append(metadataJoiner)
            append(status.language.uppercase())
        }

        val app = status.application
        if (app != null) {
            append(metadataJoiner)

            if (app.website != null) {
                withLink(
                    LinkAnnotation.Clickable(
                        tag = "app",
                        styles = linkStyles(),
                        linkInteractionListener = {
                            listener.onViewUrl(app.website)
                        }
                    )
                ) {
                    append(app.name)
                }
            } else {
                append(app.name)
            }
        }
    }
    Text(
        text = metadata,
        style = LocalPreferences.current.statusTextStyles.medium,
        color = warpdroidColors.tertiaryTextColor,
        inlineContent = mapOf(
            "visibility" to InlineTextContent(
                placeholder = Placeholder(
                    width = 18.sp,
                    height = 18.sp,
                    placeholderVerticalAlign = PlaceholderVerticalAlign.Center
                ),
                children = {
                    getVisibilityIcon(status.visibility)?.let { icon ->
                        val size = with(LocalDensity.current) { 18.sp.toDp() }
                        Icon(
                            painterResource(icon),
                            tint = warpdroidColors.tertiaryTextColor,
                            contentDescription = null,
                            modifier = Modifier.size(size)
                        )
                    }
                }
            )
        ),
        modifier = modifier
    )
}

@Composable
private fun DetailedStatistics(
    statusViewData: TweetViewData.Concrete,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    FlowRow(
        modifier = modifier,
        horizontalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text(
            text = getMetaDataText(R.plurals.retweets, statusViewData.status.retweetsCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = warpdroidColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showRetweets(statusViewData)
                }
        )
        Text(
            text = getMetaDataText(R.plurals.quotes, statusViewData.status.quotesCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = warpdroidColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showQuotes(statusViewData)
                }
        )
        Text(
            text = getMetaDataText(R.plurals.favs, statusViewData.status.likesCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = warpdroidColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showFavs(statusViewData)
                }
        )
        // Warpnet view counter, populated lazily by getTweetStats. We
        // only render it when the count is known (>0) so a missing
        // stats fetch doesn't make every detailed status read "0 Views".
        if (statusViewData.status.viewsCount > 0) {
            Text(
                text = getMetaDataText(R.plurals.views, statusViewData.status.viewsCount),
                style = LocalPreferences.current.statusTextStyles.medium,
                color = warpdroidColors.tertiaryTextColor,
            )
        }
    }
}

private val numberFormat = NumberFormat.getNumberInstance()

@Composable
@ReadOnlyComposable
private fun getMetaDataText(@PluralsRes text: Int, count: Int): AnnotatedString {
    val countString = numberFormat.format(count)
    val textString = pluralStringResource(text, count, countString)

    return buildAnnotatedString {
        append(textString)
        val countIndex = textString.indexOf(countString)
        if (countIndex != -1) {
            addStyle(SpanStyle(fontWeight = FontWeight.Bold), countIndex, countIndex + countString.length)
        }
    }
}

@StringRes
private fun getVisibilityText(visibility: Tweet.Visibility): Int? {
    return when (visibility) {
        Tweet.Visibility.PUBLIC -> R.string.description_visibility_public
        Tweet.Visibility.UNLISTED -> R.string.description_visibility_unlisted
        Tweet.Visibility.PRIVATE -> R.string.description_visibility_private
        Tweet.Visibility.DIRECT -> R.string.description_visibility_direct
        else -> null
    }
}

@DrawableRes
private fun getVisibilityIcon(visibility: Tweet.Visibility): Int? {
    return when (visibility) {
        Tweet.Visibility.PUBLIC -> R.drawable.ic_public_24dp
        Tweet.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
        Tweet.Visibility.PRIVATE -> R.drawable.ic_lock_24dp
        Tweet.Visibility.DIRECT -> R.drawable.ic_mail_24dp
        else -> null
    }
}

@PreviewLightDark
@Composable
fun DetailedTweetPreview() {
    WarpdroidPreviewTheme {
        DetailedTweet(
            statusViewData = fakeTweetViewData().copy(isDetailed = true),
            listener = noopListener,
            accounts = emptyList(),
            modifier = Modifier.background(colorScheme.background)
        )
    }
}
