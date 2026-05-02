package com.keylesspalace.tusky.ui.statuscomponents

import androidx.annotation.DrawableRes
import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.Notification
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.entity.TimelineAccount
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.unicodeWrap
import com.keylesspalace.tusky.viewdata.NotificationViewData

@Composable
fun NotificationInfo(
    notificationViewData: NotificationViewData.Concrete,
    listener: StatusActionListener
) {
    when (notificationViewData.type) {
        Notification.Type.Mention -> {
            MentionNotificationStatusInfo(notificationViewData)
        }
        Notification.Type.Poll -> {
            PollNotificationStatusInfo(notificationViewData)
        }
        Notification.Type.Status -> {
            NotificationInfo(
                icon = R.drawable.ic_notifications_active_24dp,
                iconColor = colorScheme.primary,
                text = R.string.notification_subscription_format,
                account = notificationViewData.statusViewData!!.status.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.statusViewData.status.account.id)
                }
            )
        }
        Notification.Type.Update -> {
            NotificationInfo(
                icon = R.drawable.ic_edit_24dp_filled,
                iconColor = colorScheme.primary,
                text = R.string.notification_update_format,
                account = notificationViewData.statusViewData!!.status.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.statusViewData.status.account.id)
                }
            )
        }
        Notification.Type.Favourite -> {
            NotificationInfo(
                icon = R.drawable.ic_star_24dp_filled,
                iconColor = tuskyColors.favoriteButtonActiveColor,
                text = R.string.notification_favourite_format,
                account = notificationViewData.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.account.id)
                }
            )
        }
        Notification.Type.PleromaEmojiReaction -> {
            when {
                // custom emoji
                !notificationViewData.emojiUrl.isNullOrBlank() -> NotificationInfoWithEmojiUrl(
                    emojiUrl = notificationViewData.emojiUrl,
                    text = R.string.notification_pleroma_reaction_format,
                    account = notificationViewData.account,
                    onViewAccount = {
                        listener.onViewAccount(notificationViewData.account.id)
                    }
                )
                // "builtin" emoji
                !notificationViewData.emoji.isNullOrBlank() -> NotificationInfoWithEmojiString(
                    iconEmoji = notificationViewData.emoji,
                    text = R.string.notification_pleroma_reaction_format,
                    account = notificationViewData.account,
                    onViewAccount = {
                        listener.onViewAccount(notificationViewData.account.id)
                    }
                )
                // reaction with no emoji info, fall back to star
                else -> NotificationInfo(
                    icon = R.drawable.ic_star_24dp_filled,
                    iconColor = tuskyColors.favoriteButtonActiveColor,
                    text = R.string.notification_pleroma_reaction_format,
                    account = notificationViewData.account,
                    onViewAccount = {
                        listener.onViewAccount(notificationViewData.account.id)
                    }
                )
            }
        }
        Notification.Type.Reblog -> {
            NotificationInfo(
                icon = R.drawable.ic_repeat_24dp,
                iconColor = colorScheme.primary,
                text = R.string.notification_reblog_format,
                account = notificationViewData.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.account.id)
                }
            )
        }
        Notification.Type.Quote -> {
            NotificationInfo(
                icon = R.drawable.ic_format_quote_24dp_filled,
                iconColor = colorScheme.primary,
                text = R.string.notification_quote_format,
                account = notificationViewData.statusViewData!!.status.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.statusViewData.status.account.id)
                }
            )
        }
        Notification.Type.QuotedUpdate -> {
            NotificationInfo(
                icon = R.drawable.ic_edit_24dp_filled,
                iconColor = colorScheme.primary,
                text = R.string.notification_quoted_update_format,
                account = notificationViewData.statusViewData!!.status.quote!!.quotedStatus!!.account,
                onViewAccount = {
                    listener.onViewAccount(notificationViewData.statusViewData.status.quote.quotedStatus.account.id)
                }
            )
        }
        else -> {
            // not used for other types of notifications
        }
    }
}

@Composable
private fun MentionNotificationStatusInfo(
    notificationViewData: NotificationViewData.Concrete
) {
    val activeAccount = LocalAccount.current
    Row(
        verticalAlignment = Alignment.CenterVertically
    ) {
        Spacer(modifier = Modifier.width(52.dp))
        Icon(
            painter = if (notificationViewData.statusViewData!!.status.inReplyToAccountId == activeAccount?.accountId) {
                painterResource(R.drawable.ic_reply_18dp)
            } else {
                painterResource(R.drawable.ic_email_alternate_18dp)
            },
            tint = tuskyColors.tertiaryTextColor,
            contentDescription = null
        )
        Spacer(modifier = Modifier.width(6.dp))
        Text(
            text = if (notificationViewData.statusViewData.status.inReplyToAccountId == activeAccount?.accountId) {
                if (notificationViewData.statusViewData.status.visibility == Status.Visibility.DIRECT) {
                    stringResource(R.string.notification_info_private_reply)
                } else {
                    stringResource(R.string.notification_info_reply)
                }
            } else {
                if (notificationViewData.statusViewData.status.visibility == Status.Visibility.DIRECT) {
                    stringResource(R.string.notification_info_private_mention)
                } else {
                    stringResource(R.string.notification_info_mention)
                }
            },
            color = tuskyColors.tertiaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium
        )
    }
}

@Composable
private fun PollNotificationStatusInfo(
    notificationViewData: NotificationViewData.Concrete
) {
    val activeAccount = LocalAccount.current

    Row(
        verticalAlignment = Alignment.CenterVertically
    ) {
        Spacer(modifier = Modifier.width(42.dp))
        Icon(
            painter = painterResource(R.drawable.ic_insert_chart_24dp_filled),
            tint = colorScheme.primary,
            contentDescription = null
        )
        Spacer(modifier = Modifier.width(10.dp))
        Text(
            text = if (notificationViewData.statusViewData?.status?.account?.id == activeAccount?.accountId) {
                stringResource(R.string.poll_ended_created)
            } else {
                stringResource(R.string.poll_ended_voted)
            },
            color = tuskyColors.secondaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium
        )
    }
}

@Composable
private fun NotificationInfo(
    @DrawableRes icon: Int,
    iconColor: Color,
    @StringRes text: Int,
    account: TimelineAccount,
    onViewAccount: () -> Unit
) {
    val displayName = account.name.unicodeWrap()
    val text = stringResource(text)

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .padding(start = 42.dp)
            .clickable {
                onViewAccount()
            }
    ) {
        Icon(
            painter = painterResource(icon),
            tint = iconColor,
            contentDescription = null
        )
        Spacer(modifier = Modifier.width(10.dp))
        Text(
            text = buildAnnotatedString {
                val emojifiedText = text.format(displayName).emojify(account.emojis)
                val startIndex = text.indexOf($$"%1$s")
                append(emojifiedText)
                addStyle(
                    SpanStyle(fontWeight = FontWeight.Bold),
                    start = startIndex,
                    end = emojifiedText.length - (text.length - startIndex) + $$"%1$s".length
                )
            },
            color = tuskyColors.secondaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            inlineContent = account.emojis.toInlineContent()
        )
    }
}

@Composable
private fun NotificationInfoWithEmojiString(
    iconEmoji: String,
    @StringRes text: Int,
    account: TimelineAccount,
    onViewAccount: () -> Unit
) {
    val displayName = account.name.unicodeWrap()
    val text = stringResource(text)

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .padding(start = 42.dp)
            .clickable {
                onViewAccount()
            }
    ) {
        Text(iconEmoji)
        Spacer(modifier = Modifier.width(10.dp))
        Text(
            text = buildAnnotatedString {
                val emojifiedText = text.format(displayName).emojify(account.emojis)
                val startIndex = text.indexOf($$"%1$s")
                append(emojifiedText)
                addStyle(
                    SpanStyle(fontWeight = FontWeight.Bold),
                    start = startIndex,
                    end = emojifiedText.length - (text.length - startIndex) + $$"%1$s".length
                )
            },
            color = tuskyColors.secondaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            inlineContent = account.emojis.toInlineContent()
        )
    }
}

@Composable
private fun NotificationInfoWithEmojiUrl(
    emojiUrl: String,
    @StringRes text: Int,
    account: TimelineAccount,
    onViewAccount: () -> Unit
) {
    val displayName = account.name.unicodeWrap()
    val text = stringResource(text)

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .padding(start = 42.dp)
            .clickable {
                onViewAccount()
            }
    ) {
        AsyncImage(
            model = emojiUrl,
            contentDescription = null,
            contentScale = ContentScale.Fit
        )
        Spacer(modifier = Modifier.width(10.dp))
        Text(
            text = buildAnnotatedString {
                val emojifiedText = text.format(displayName).emojify(account.emojis)
                val startIndex = text.indexOf($$"%1$s")
                append(emojifiedText)
                addStyle(
                    SpanStyle(fontWeight = FontWeight.Bold),
                    start = startIndex,
                    end = emojifiedText.length - (text.length - startIndex) + $$"%1$s".length
                )
            },
            color = tuskyColors.secondaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            inlineContent = account.emojis.toInlineContent()
        )
    }
}
