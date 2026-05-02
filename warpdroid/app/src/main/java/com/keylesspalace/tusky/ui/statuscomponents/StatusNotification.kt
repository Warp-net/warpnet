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
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Icon
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
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.Notification
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyButtonSize
import com.keylesspalace.tusky.ui.TuskyOutlinedButton
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeStatusViewData
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeTimelineAccount
import com.keylesspalace.tusky.ui.statuscomponents.fake.noopListener
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.mastodonHtmlText
import com.keylesspalace.tusky.ui.statuscomponents.text.toAnnotatedString
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.viewdata.NotificationViewData

/**
 * Notification when someone favourited or boosted a post
 */
@Composable
fun StatusNotification(
    notificationViewData: NotificationViewData.Concrete,
    listener: StatusActionListener,
    modifier: Modifier = Modifier
) {
    val statusViewData = notificationViewData.statusViewData ?: return
    val status = statusViewData.status

    Column(
        modifier = modifier.clickable {
            listener.onViewThread(statusViewData)
        }
            .padding(top = 8.dp)
    ) {
        NotificationInfo(
            notificationViewData = notificationViewData,
            listener = listener
        )

        Row(
            modifier = Modifier.padding(start = 14.dp, top = 6.dp, end = 14.dp)
        ) {
            Avatar(
                url = status.account.avatar,
                staticUrl = status.account.staticAvatar,
                isBot = status.account.bot,
                boostedAvatarUrl = notificationViewData.account.avatar,
                staticBoostedAvatarUrl = notificationViewData.account.staticAvatar,
                onOpenProfile = {
                    listener.onViewAccount(status.account.id)
                },
                modifier = Modifier.clearAndSetSemantics {
                    hideFromAccessibility()
                }
            )

            Spacer(modifier = Modifier.width(16.dp))

            Column(
                modifier = Modifier.weight(1f)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    val username = stringResource(R.string.post_username_format, status.account.username)

                    Row(
                        modifier = Modifier
                            .weight(1f)
                            .clearAndSetSemantics {
                                contentDescription = "${status.account.name} $username"
                            }
                            .clickable {
                                listener.onViewAccount(status.account.id)
                            },
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        Text(
                            text = status.account.name.emojify(status.account.emojis),
                            fontWeight = FontWeight.Bold,
                            color = tuskyColors.tertiaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            inlineContent = status.account.emojis.toInlineContent()
                        )
                        Text(
                            text = username,
                            color = tuskyColors.tertiaryTextColor,
                            style = LocalPreferences.current.statusTextStyles.medium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }

                    Timestamp(
                        date = status.createdAt,
                        isEdited = status.editedAt != null,
                        textColor = tuskyColors.tertiaryTextColor,
                        modifier = Modifier.padding(start = 4.dp)
                    )
                }

                var isExpanded by remember(statusViewData.isExpanded) { mutableStateOf(statusViewData.isExpanded) }

                if (status.spoilerText.isNotEmpty()) {
                    Text(
                        text = status.spoilerText.emojify(status.emojis),
                        color = tuskyColors.tertiaryTextColor,
                        style = LocalPreferences.current.statusTextStyles.medium,
                        inlineContent = status.emojis.toInlineContent(),
                        modifier = Modifier.padding(top = 6.dp)
                    )
                    TuskyOutlinedButton(
                        text = if (isExpanded) {
                            stringResource(R.string.post_content_warning_show_less)
                        } else {
                            stringResource(R.string.post_content_warning_show_more)
                        },
                        onClick = {
                            isExpanded = !isExpanded
                            listener.onExpandedChange(statusViewData, isExpanded)
                        },
                        size = TuskyButtonSize.Small,
                        modifier = Modifier
                            .widthIn(min = 150.dp)
                            .padding(top = 6.dp)

                    )
                }

                var isCollapsed by remember(statusViewData.isCollapsed) { mutableStateOf(statusViewData.isCollapsed) }

                val linkColor = tuskyColors.tertiaryTextColor

                val activeLinkStyle = SpanStyle(color = linkColor, background = linkColor.copy(alpha = 0.25f))
                val linkStyles = TextLinkStyles(
                    style = SpanStyle(color = linkColor),
                    focusedStyle = activeLinkStyle,
                    hoveredStyle = activeLinkStyle,
                    pressedStyle = activeLinkStyle
                )

                val (content, _) = if (isExpanded || status.spoilerText.isEmpty()) {
                    mastodonHtmlText(
                        status = statusViewData,
                        onMentionClick = { accountId -> listener.onViewAccount(accountId) },
                        onHashtagClick = { tag -> listener.onViewTag(tag) },
                        onUrlClick = { url -> listener.onViewUrl(url) },
                        splitOffTrailingHashtags = false,
                        linkStyles = linkStyles
                    )
                } else {
                    status.mentions.toAnnotatedString(
                        onMentionClick = { accountId -> listener.onViewAccount(accountId) },
                        linkStyles = linkStyles
                    ) to emptyList()
                }

                StatusText(
                    content = content,
                    status = statusViewData,
                    isCollapsed = isCollapsed,
                    isExpanded = isExpanded,
                    textColor = tuskyColors.tertiaryTextColor,
                    onContentCollapsedChange = {
                        isCollapsed = !isCollapsed
                        listener.onContentCollapsedChange(statusViewData, isCollapsed)
                    },
                    modifier = Modifier.padding(top = 6.dp)
                )

                if (status.spoilerText.isEmpty() || isExpanded) {
                    if (status.attachments.isNotEmpty()) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.padding(top = 8.dp)
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_attach_file_24dp),
                                tint = tuskyColors.tertiaryTextColor,
                                contentDescription = null,
                                modifier = Modifier.padding(end = 4.dp)
                            )
                            val attachmentCount = status.attachments.size
                            Text(
                                text = pluralStringResource(R.plurals.media_attachments, attachmentCount, attachmentCount),
                                style = LocalPreferences.current.statusTextStyles.medium,
                                color = tuskyColors.tertiaryTextColor
                            )
                        }
                    }

                    if (status.poll != null) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.padding(top = 8.dp)
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_insert_chart_24dp),
                                tint = tuskyColors.tertiaryTextColor,
                                contentDescription = null,
                                modifier = Modifier.padding(end = 4.dp)
                            )
                            Text(
                                text = stringResource(R.string.poll),
                                style = LocalPreferences.current.statusTextStyles.medium,
                                color = tuskyColors.tertiaryTextColor
                            )
                        }
                    }

                    if (status.quote != null) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.padding(top = 8.dp)
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_format_quote_24dp),
                                tint = tuskyColors.tertiaryTextColor,
                                contentDescription = null,
                                modifier = Modifier.padding(end = 4.dp)
                            )
                            Text(
                                text = stringResource(R.string.quote),
                                style = LocalPreferences.current.statusTextStyles.medium,
                                color = tuskyColors.tertiaryTextColor
                            )
                        }
                    }
                }
            }
        }

        Spacer(modifier = Modifier.height(8.dp))
        // TODO: Enable this once this Composable is no longer used in a RecyclerView
        // HorizontalDivider()
    }
}

@PreviewLightDark
@Composable
fun StatusNotificationPreview() {
    TuskyPreviewTheme {
        StatusNotification(
            notificationViewData = NotificationViewData.Concrete(
                id = "1",
                type = Notification.Type.Favourite,
                account = fakeTimelineAccount,
                statusViewData = fakeStatusViewData(),
                report = null,
                event = null,
                moderationWarning = null,
                emoji = null,
                emojiUrl = null,
            ),
            listener = noopListener,
            modifier = Modifier.background(colorScheme.background)
        )
    }
}
