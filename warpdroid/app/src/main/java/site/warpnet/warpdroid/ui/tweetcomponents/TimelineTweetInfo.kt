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
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.interfaces.TweetActionListener
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.text.emojify
import site.warpnet.warpdroid.ui.tweetcomponents.text.toInlineContent
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.unicodeWrap
import site.warpnet.warpdroid.viewdata.TweetViewData
import kotlin.collections.orEmpty

@Composable
fun TimelineStatusInfo(
    statusViewData: TweetViewData.Concrete,
    listener: TweetActionListener,
) {
    var onClick: (() -> Unit)? = null

    @DrawableRes var icon: Int = -1

    var text: AnnotatedString? = null
    var emojis: List<Emoji> = emptyList()

    val retweetgingStatus = statusViewData.retweetgingStatus

    if (retweetgingStatus != null) {
        onClick = { listener.onViewAccount(retweetgingStatus.account.id) }
        icon = R.drawable.ic_repeat_18dp
        text = stringResource(R.string.post_retweeted_format, statusViewData.retweetgingStatus?.account?.name.orEmpty().unicodeWrap())
            .emojify(statusViewData.retweetgingStatus?.account?.emojis.orEmpty())
        emojis = retweetgingStatus.account.emojis
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
                tint = warpdroidColors.tertiaryTextColor,
                contentDescription = null
            )
            Spacer(Modifier.width(6.dp))
            Text(
                text = text,
                color = warpdroidColors.tertiaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                inlineContent = emojis.toInlineContent()
            )
        }
    }
}
