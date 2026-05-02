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

import androidx.activity.compose.LocalActivity
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.intl.Locale
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import com.keylesspalace.tusky.BaseActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.Translation
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyTextButton
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.copyToClipboard
import com.keylesspalace.tusky.util.shareStatusContent
import com.keylesspalace.tusky.util.shareStatusLink
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewdata.TranslationViewData

@Composable
fun StatusMoreMenu(
    viewData: StatusViewData.Concrete,
    expanded: Boolean,
    onDismissRequest: () -> Unit,
    translationEnabled: Boolean,
    accounts: List<AccountEntity>,
    listener: StatusActionListener
) {
    var shareMenuVisible by remember { mutableStateOf(false) }
    var showOpenAsDialog by remember { mutableStateOf(false) }

    val status = viewData.actionable

    val activeAccount = LocalAccount.current
    val isOwnStatus = status.account.id == activeAccount?.accountId

    val context = LocalContext.current

    if (showOpenAsDialog) {
        val activity = LocalActivity.current as? BaseActivity?
        AlertDialog(
            onDismissRequest = { showOpenAsDialog = false },
            confirmButton = {
                TuskyTextButton(
                    text = stringResource(android.R.string.cancel),
                    onClick = { showOpenAsDialog = false }
                )
            },
            text = {
                Column(
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    val currentAccount = LocalAccount.current
                    accounts.forEach { account ->
                        if (account.id == currentAccount?.id) {
                            return@forEach
                        }
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.clickable {
                                activity?.openAsAccount(status.url!!, account)
                            }
                        ) {
                            Avatar(
                                url = account.profilePictureUrl,
                                staticUrl = account.staticProfilePictureUrl,
                                isBot = false,
                                boostedAvatarUrl = null,
                                staticBoostedAvatarUrl = null,
                                onOpenProfile = null,
                            )
                            Spacer(modifier = Modifier.width(8.dp))
                            Column {
                                Text(
                                    text = account.displayName.emojify(account.emojis),
                                    fontWeight = FontWeight.Bold,
                                    color = tuskyColors.primaryTextColor,
                                    style = LocalPreferences.current.statusTextStyles.medium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                    inlineContent = account.emojis.toInlineContent()
                                )
                                Text(
                                    text = account.fullName,
                                    color = tuskyColors.secondaryTextColor,
                                    style = LocalPreferences.current.statusTextStyles.medium,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                            }
                        }
                    }
                }
            }
        )
    }

    val confirmationDialogState = rememberDialogState()
    Dialog(state = confirmationDialogState)

    if (shareMenuVisible) {
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = {
                shareMenuVisible = false
                onDismissRequest()
            }
        ) {
            Text(
                text = stringResource(R.string.action_share),
                color = tuskyColors.tertiaryTextColor,
                textAlign = TextAlign.Center,
                modifier = Modifier
                    .padding(horizontal = 12.dp, vertical = 8.dp)
            )
            DropdownMenuItem(
                text = { Text(stringResource(R.string.post_share_link)) },
                onClick = {
                    shareMenuVisible = false
                    onDismissRequest()
                    context.shareStatusLink(viewData)
                }
            )
            DropdownMenuItem(
                text = { Text(stringResource(R.string.post_share_content)) },
                onClick = {
                    shareMenuVisible = false
                    onDismissRequest()
                    context.shareStatusContent(viewData)
                }
            )
        }
    } else {
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = onDismissRequest
        ) {
            val translation: Translation? = (viewData.translation as? TranslationViewData.Loaded)?.data
            val locale = Locale.current

            DropdownMenuItem(
                text = { Text(stringResource(R.string.action_share)) },
                onClick = {
                    shareMenuVisible = true
                }
            )
            if (status.url != null) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_copy_link)) },
                    onClick = {
                        onDismissRequest()
                        context.copyToClipboard(status.url, context.getString(R.string.url_copied))
                    }
                )
            }
            val translateable = translationEnabled &&
                !status.language.equals(locale.language, ignoreCase = true) &&
                (status.visibility == Status.Visibility.PUBLIC || status.visibility == Status.Visibility.UNLISTED)
            val reFilterable = !viewData.filterActive && viewData.filter?.action == Filter.Action.WARN
            val otherAccountsAvailable = accounts.size > 1

            if (translateable || reFilterable || otherAccountsAvailable) {
                HorizontalDivider()
            }

            if (translateable) {
                if (translation == null) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.action_translate)) },
                        onClick = {
                            onDismissRequest()
                            listener.onTranslate(viewData)
                        }
                    )
                } else {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.action_show_original)) },
                        onClick = {
                            onDismissRequest()
                            listener.onUntranslate(viewData)
                        }
                    )
                }
            }

            if (reFilterable) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_rehide_status)) },
                    onClick = {
                        onDismissRequest()
                        listener.changeFilter(viewData, filtered = true)
                    }
                )
            }

            if (accounts.size == 2) {
                val currentAccount = LocalAccount.current
                val otherAccount = accounts.first { account ->
                    account.id != currentAccount?.id
                }
                val activity = LocalActivity.current as? BaseActivity?
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_open_as, otherAccount.fullName)) },
                    onClick = {
                        onDismissRequest()
                        activity?.openAsAccount(status.url!!, otherAccount)
                    }
                )
            } else if (accounts.size > 2) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_open_as, "...")) },
                    onClick = {
                        onDismissRequest()
                        showOpenAsDialog = true
                    }
                )
            }

            HorizontalDivider()

            if (isOwnStatus && status.visibility == Status.Visibility.PRIVATE) {
                if (status.reblogged) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.unreblog_private)) },
                        onClick = {
                            onDismissRequest()
                            listener.onReblog(viewData, false, Status.Visibility.PRIVATE, null)
                        }
                    )
                } else {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.reblog_private)) },
                        onClick = {
                            onDismissRequest()
                            listener.onReblog(viewData, true, Status.Visibility.PRIVATE, null)
                        }
                    )
                }
            }
            if (isOwnStatus && (status.visibility == Status.Visibility.PUBLIC || status.visibility == Status.Visibility.UNLISTED)) {
                if (status.pinned) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.unpin_action)) },
                        onClick = {
                            onDismissRequest()
                            listener.onPin(viewData, false)
                        }
                    )
                } else {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.pin_action)) },
                        onClick = {
                            onDismissRequest()
                            listener.onPin(viewData, true)
                        }
                    )
                }
            }
            if (isOwnStatus || accountIsInMentions(activeAccount, status.mentions)) {
                DropdownMenuItem(
                    text = {
                        if (status.muted) {
                            Text(stringResource(R.string.action_unmute_conversation))
                        } else {
                            Text(stringResource(R.string.action_mute_conversation))
                        }
                    },
                    onClick = {
                        onDismissRequest()
                        listener.onMuteConversation(viewData, !status.muted)
                    }
                )
            }

            if (!isOwnStatus) {
                status.quote?.quotedStatus?.let { quotedStatus ->
                    if (quotedStatus.account.id == activeAccount?.accountId) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.action_remove_quote)) },
                            onClick = {
                                onDismissRequest()
                                confirmationDialogState.showRemovePostDialog("@${quotedStatus.account.username}") {
                                    listener.removeQuote(viewData)
                                }
                            }
                        )
                    }
                }

                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_mute)) },
                    onClick = {
                        onDismissRequest()
                        confirmationDialogState.showConfirmMuteDialog(status.account.username) { hideNotifications, duration ->
                            listener.onMute(status.account.id, hideNotifications, duration)
                        }
                    }
                )

                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_block)) },
                    onClick = {
                        onDismissRequest()
                        confirmationDialogState.showBlockAccountDialog(status.account.username) {
                            listener.onBlock(status.account.id)
                        }
                    }
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_report)) },
                    onClick = {
                        onDismissRequest()
                        listener.onReport(viewData)
                    }
                )
            } else {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_edit)) },
                    onClick = {
                        onDismissRequest()
                        listener.onEdit(viewData)
                    }
                )

                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_delete)) },
                    onClick = {
                        onDismissRequest()
                        confirmationDialogState.showDeleteStatusDialog {
                            listener.onDelete(viewData)
                        }
                    }
                )
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_delete_and_redraft)) },
                    onClick = {
                        onDismissRequest()
                        confirmationDialogState.showConfirmRedraftDialog {
                            listener.onRedraft(viewData)
                        }
                    }
                )
            }
        }
    }
}

private fun accountIsInMentions(
    account: AccountEntity?,
    mentions: List<Status.Mention>
): Boolean {
    return mentions.any { mention ->
        account?.username == mention.username && account.domain == mention.url.toUri().host
    }
}
