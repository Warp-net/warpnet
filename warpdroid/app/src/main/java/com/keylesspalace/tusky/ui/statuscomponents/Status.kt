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
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeStatusViewData
import com.keylesspalace.tusky.ui.statuscomponents.fake.noopListener
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.viewdata.StatusViewData

@Composable
fun Status(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener,
    translationEnabled: Boolean,
    accounts: List<AccountEntity>,
    modifier: Modifier = Modifier,
    statusInfo: (@Composable () -> Unit) = {},
    // Todo: remove once no longer needed for notifications
    showDivider: Boolean = true
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
            .padding(top = 8.dp)
    ) {
        statusInfo()

        Row(
            modifier = Modifier.padding(start = 14.dp, top = 6.dp, end = 14.dp)
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
                modifier = Modifier.weight(1f)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    val username = stringResource(R.string.post_username_format, status.account.username)
                    Box(
                        modifier = Modifier
                            .weight(1f)
                    ) {
                        Row(
                            modifier = Modifier
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

                    Timestamp(
                        date = status.createdAt,
                        isEdited = status.editedAt != null,
                        textColor = tuskyColors.secondaryTextColor,
                        modifier = Modifier.padding(start = 4.dp)
                    )
                }

                StatusContent(
                    statusViewData = statusViewData,
                    listener = listener
                )
            }
        }
        StatusButtons(
            statusViewData = statusViewData,
            listener = listener,
            translationEnabled = translationEnabled,
            accounts = accounts,
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 72.dp, end = 8.dp, top = 2.dp)
        )
        if (showDivider) {
            HorizontalDivider()
        }
    }
}

@PreviewLightDark
@Composable
fun StatusPreview() {
    TuskyPreviewTheme {
        Status(
            statusViewData = fakeStatusViewData(),
            listener = noopListener,
            translationEnabled = false,
            accounts = emptyList(),
            modifier = Modifier.background(colorScheme.background)
        )
    }
}
