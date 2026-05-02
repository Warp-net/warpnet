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

import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withLink
import com.keylesspalace.tusky.interfaces.StatusActionListener

@Composable
fun Hashtags(
    hashtags: List<String>,
    singleLine: Boolean,
    style: TextStyle,
    listener: StatusActionListener,
    modifier: Modifier = Modifier
) {
    val primaryColor = colorScheme.primary
    val activeLinkStyle = SpanStyle(color = primaryColor, background = primaryColor.copy(alpha = 0.25f))
    val linkStyles = TextLinkStyles(
        style = SpanStyle(color = primaryColor),
        focusedStyle = activeLinkStyle,
        hoveredStyle = activeLinkStyle,
        pressedStyle = activeLinkStyle
    )

    val content = buildAnnotatedString {
        hashtags.forEach { tag ->
            withLink(
                LinkAnnotation.Clickable(
                    tag = tag,
                    styles = linkStyles,
                    linkInteractionListener = {
                        listener.onViewTag(tag)
                    }
                )
            ) {
                append("#")
                append(tag)
            }
            append(" ")
        }
    }

    Text(
        text = content,
        style = style,
        maxLines = if (singleLine) {
            1
        } else {
            Int.MAX_VALUE
        },
        overflow = TextOverflow.Ellipsis,
        modifier = modifier
    )
}
