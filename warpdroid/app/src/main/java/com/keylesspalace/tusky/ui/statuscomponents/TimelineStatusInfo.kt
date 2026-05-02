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
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.unicodeWrap
import com.keylesspalace.tusky.viewdata.StatusViewData
import kotlin.collections.orEmpty

@Composable
fun TimelineStatusInfo(
    statusViewData: StatusViewData.Concrete,
    listener: StatusActionListener,
) {
    var onClick: (() -> Unit)? = null

    @DrawableRes var icon: Int = -1

    var text: AnnotatedString? = null
    var emojis: List<Emoji> = emptyList()

    val rebloggingStatus = statusViewData.rebloggingStatus

    if (rebloggingStatus != null) {
        onClick = { listener.onViewAccount(rebloggingStatus.account.id) }
        icon = R.drawable.ic_repeat_18dp
        text = stringResource(R.string.post_boosted_format, statusViewData.rebloggingStatus?.account?.name.orEmpty().unicodeWrap())
            .emojify(statusViewData.rebloggingStatus?.account?.emojis.orEmpty())
        emojis = rebloggingStatus.account.emojis
    } else if (statusViewData.isReply) {
        icon = R.drawable.ic_reply_18dp
        text = if (statusViewData.isSelfReply) {
            AnnotatedString(stringResource(R.string.post_replied_self))
        } else if (statusViewData.repliedToAccount != null) {
            onClick = {
                listener.onViewAccount(statusViewData.repliedToAccount.id)
            }
            stringResource(R.string.post_replied_format, statusViewData.repliedToAccount.name.unicodeWrap())
                .emojify(statusViewData.repliedToAccount.emojis)
        } else {
            AnnotatedString(stringResource(R.string.post_replied))
        }
        emojis = statusViewData.repliedToAccount?.emojis.orEmpty()
    }

    if (text != null && icon != -1) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .padding(start = 52.dp)
                .clearAndSetSemantics {
                    contentDescription = text.toString()
                }
                .run {
                    if (onClick != null) {
                        clickable {
                            onClick()
                        }
                    } else {
                        this
                    }
                }
        ) {
            Icon(
                painter = painterResource(icon),
                tint = tuskyColors.tertiaryTextColor,
                contentDescription = null
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = text,
                color = tuskyColors.tertiaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                inlineContent = emojis.toInlineContent()
            )
        }
    }
}
