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

import androidx.annotation.VisibleForTesting
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush.Companion.verticalGradient
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ui.WarpdroidButtonSize
import site.warpnet.warpdroid.ui.WarpdroidOutlinedButton
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.tweetcomponents.text.INLINE_CONTENT_TAG
import site.warpnet.warpdroid.ui.tweetcomponents.text.background.QuotePainter
import site.warpnet.warpdroid.ui.tweetcomponents.text.background.TextBackgroundPainters
import site.warpnet.warpdroid.ui.tweetcomponents.text.background.drawBehind
import site.warpnet.warpdroid.ui.tweetcomponents.text.html.AnnotatedStringHtmlHandler.Companion.LINK_ICON_ID
import site.warpnet.warpdroid.ui.tweetcomponents.text.toInlineContent
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.viewdata.TweetViewData

@Composable
fun ColumnScope.TweetText(
    content: AnnotatedString,
    status: TweetViewData.Concrete,
    isCollapsed: Boolean,
    isExpanded: Boolean,
    textColor: Color,
    onContentCollapsedChange: () -> Unit,
    modifier: Modifier = Modifier
) {
    if (content.isEmpty()) {
        return
    }

    val isCollapsible = content.lines().size > 14 || content.visibleLength() > 800

    Box(modifier = modifier) {
        val quoteColor = warpdroidColors.tertiaryTextColor

        val backgroundPainters = remember {
            TextBackgroundPainters(
                QuotePainter(quoteColor)
            )
        }

        val textStyle = if (status.isDetailed) {
            LocalPreferences.current.statusTextStyles.large
        } else {
            LocalPreferences.current.statusTextStyles.medium
        }

        Text(
            modifier = modifier.drawBehind(backgroundPainters),
            text = content,
            color = textColor,
            maxLines = if (!isCollapsible || !isCollapsed || status.isDetailed) Int.MAX_VALUE else 10,
            onTextLayout = { result ->
                backgroundPainters.onTextLayout(result)
            },
            style = textStyle,
            inlineContent = status.actionable.emojis.toInlineContent()
                .plus(
                    LINK_ICON_ID to InlineTextContent(
                        placeholder = Placeholder(
                            width = 18.sp,
                            height = 18.sp,
                            placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter
                        ),
                        children = {
                            Icon(
                                painter = painterResource(R.drawable.ic_open_in_new_24dp),
                                tint = colorScheme.primary,
                                contentDescription = null
                            )
                        }
                    )
                )
        )

        if ((isExpanded || status.actionable.spoilerText.isEmpty()) && isCollapsible && isCollapsed && !status.isDetailed) {
            // Fading Edge overlay
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(80.dp)
                    .align(Alignment.BottomCenter)
                    .background(
                        verticalGradient(
                            colors = listOf(Color.Transparent, colorScheme.background)
                        )
                    )
            )
        }
    }
    if ((isExpanded || status.actionable.spoilerText.isEmpty()) && isCollapsible && !status.isDetailed) {
        WarpdroidOutlinedButton(
            text = if (isCollapsed) {
                stringResource(R.string.post_content_show_more)
            } else {
                stringResource(R.string.post_content_show_less)
            },
            onClick = {
                onContentCollapsedChange()
            },
            modifier = Modifier
                .align(Alignment.CenterHorizontally)
                .padding(top = 6.dp)
                .widthIn(min = 112.dp)
                .semantics { hideFromAccessibility() },
            size = WarpdroidButtonSize.Small
        )
    }
}

/**
 * The length of the text as it is rendered. Custom emojis (and the link symbol) are only counted as 2 characters.
 */
@VisibleForTesting
internal fun AnnotatedString.visibleLength(): Int {
    val emojiAnnotations = getStringAnnotations(INLINE_CONTENT_TAG, 0, length)
    val emojiTextLength = emojiAnnotations.sumOf { annotatedStringRange ->
        annotatedStringRange.end - annotatedStringRange.start
    }
    return length - emojiTextLength + emojiAnnotations.size * 2
}
