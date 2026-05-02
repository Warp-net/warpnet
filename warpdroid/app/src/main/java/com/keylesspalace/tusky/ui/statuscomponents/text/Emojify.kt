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

package com.keylesspalace.tusky.ui.statuscomponents.text

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.ui.preferences.LocalPreferences

/**
 * Returns an [AnnotatedString] with the same content as [this] and all contained custom [emojis] marked as inline content.
 * In order for the emojis to be rendered, they must also be set as inlineContent on the [androidx.compose.material3.Text].
 * @see toInlineContent
 */
@Composable
fun String.emojify(emojis: List<Emoji>): AnnotatedString {
    return remember(this, emojis) {
        buildAnnotatedString {
            append(this@emojify)

            // Iceshrimp may send duplicate emojis https://codeberg.org/tusky/Tusky/issues/5474
            emojis
                .distinctBy { emoji -> emoji.shortcode }
                .forEach { (shortcode) ->
                    val pattern = ":$shortcode:"
                    var start = indexOf(pattern)

                    while (start != -1) {
                        val end = start + pattern.length

                        addStringAnnotation(
                            tag = INLINE_CONTENT_TAG,
                            annotation = shortcode,
                            start = start,
                            end = end
                        )

                        start = indexOf(pattern, end)
                    }
                }
        }
    }
}

@Composable
fun List<Emoji>.toInlineContent(): Map<String, InlineTextContent> {
    val animateCustomEmojis = LocalPreferences.current.animateCustomEmojis
    return remember(this, animateCustomEmojis) {
        associate { emoji ->
            emoji.shortcode to InlineTextContent(
                placeholder = Placeholder(
                    width = 22.sp,
                    height = 22.sp,
                    placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter
                ),
                children = {
                    AsyncImage(
                        modifier = Modifier.fillMaxSize(),
                        model = if (animateCustomEmojis) emoji.url else emoji.staticUrl,
                        contentDescription = null,
                        contentScale = ContentScale.Fit
                    )
                }
            )
        }
    }
}

// internal tag used by Compose to identify inline content
// TODO investigate a better way to do this,
//  hardcoding an internal androidx identifier is not a good idea
const val INLINE_CONTENT_TAG = "androidx.compose.foundation.text.inlineContent"
