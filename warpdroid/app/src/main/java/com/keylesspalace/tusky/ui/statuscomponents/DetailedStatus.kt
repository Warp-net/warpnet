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
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeStatusViewData
import com.keylesspalace.tusky.ui.statuscomponents.fake.noopListener
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.linkStyles
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.showFavs
import com.keylesspalace.tusky.util.showQuotes
import com.keylesspalace.tusky.util.showReblogs
import com.keylesspalace.tusky.viewdata.StatusViewData
import java.text.DateFormat
import java.text.NumberFormat

@Composable
fun DetailedStatus(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener,
    translationEnabled: Boolean,
    accounts: List<AccountEntity>,
    showEdits: () -> Unit,
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
                        boostedAvatarUrl = statusViewData.rebloggedAvatar,
                        staticBoostedAvatarUrl = statusViewData.staticRebloggedAvatar,
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
                            color = tuskyColors.primaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            inlineContent = status.account.emojis.toInlineContent()
                        )
                        Text(
                            text = username,
                            color = tuskyColors.secondaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
                StatusContent(
                    statusViewData = statusViewData,
                    listener = listener
                )

                DetailedMetadata(
                    statusViewData = statusViewData,
                    listener = listener,
                    showEdits = showEdits,
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

        StatusButtons(
            statusViewData = statusViewData,
            listener = listener,
            translationEnabled = translationEnabled,
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
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener,
    showEdits: () -> Unit,
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
            val editedAtString = stringResource(R.string.post_edited, dateFormat.format(editedAt))

            append(metadataJoiner)

            withLink(
                LinkAnnotation.Clickable(
                    tag = "edited",
                    styles = linkStyles(),
                    linkInteractionListener = {
                        showEdits()
                    }
                )
            ) {
                append(editedAtString)
            }
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
        color = tuskyColors.tertiaryTextColor,
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
                            tint = tuskyColors.tertiaryTextColor,
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
    statusViewData: StatusViewData.Concrete,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    FlowRow(
        modifier = modifier,
        horizontalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text(
            text = getMetaDataText(R.plurals.reblogs, statusViewData.status.reblogsCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = tuskyColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showReblogs(statusViewData)
                }
        )
        Text(
            text = getMetaDataText(R.plurals.quotes, statusViewData.status.quotesCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = tuskyColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showQuotes(statusViewData)
                }
        )
        Text(
            text = getMetaDataText(R.plurals.favs, statusViewData.status.favouritesCount),
            style = LocalPreferences.current.statusTextStyles.medium,
            color = tuskyColors.tertiaryTextColor,
            modifier = Modifier
                .clickable {
                    context.showFavs(statusViewData)
                }
        )
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
private fun getVisibilityText(visibility: Status.Visibility): Int? {
    return when (visibility) {
        Status.Visibility.PUBLIC -> R.string.description_visibility_public
        Status.Visibility.UNLISTED -> R.string.description_visibility_unlisted
        Status.Visibility.PRIVATE -> R.string.description_visibility_private
        Status.Visibility.DIRECT -> R.string.description_visibility_direct
        else -> null
    }
}

@DrawableRes
private fun getVisibilityIcon(visibility: Status.Visibility): Int? {
    return when (visibility) {
        Status.Visibility.PUBLIC -> R.drawable.ic_public_24dp
        Status.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
        Status.Visibility.PRIVATE -> R.drawable.ic_lock_24dp
        Status.Visibility.DIRECT -> R.drawable.ic_mail_24dp
        else -> null
    }
}

@PreviewLightDark
@Composable
fun DetailedStatusPreview() {
    TuskyPreviewTheme {
        DetailedStatus(
            statusViewData = fakeStatusViewData().copy(isDetailed = true),
            listener = noopListener,
            translationEnabled = false,
            accounts = emptyList(),
            showEdits = { },
            modifier = Modifier.background(colorScheme.background)
        )
    }
}
