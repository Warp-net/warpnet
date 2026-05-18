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

package site.warpnet.warpdroid.ui.tweetcomponents.text

import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.runtime.Composable
import androidx.compose.ui.text.AnnotatedString
import site.warpnet.warpdroid.entity.Emoji

// Warpnet has no custom-shortcode emoji concept. The Mastodon-style
// `:shortcode:` rendering pipeline was the worst offender for transition
// jank — Compose recomposed AsyncImage per inline emoji on every Text
// remeasure. The helpers stay as no-ops so call sites keep compiling.

@Composable
@Suppress("UNUSED_PARAMETER")
fun String.emojify(emojis: List<Emoji>): AnnotatedString = AnnotatedString(this)

@Composable
@Suppress("UNUSED_PARAMETER")
fun List<Emoji>.toInlineContent(): Map<String, InlineTextContent> = emptyMap()
