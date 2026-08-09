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

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.formatNumber

/**
 * DEFAULT_REACTION mirrors the node's `domain.DefaultReaction`: the emoji a
 * like carries when the client names none, and what every like made before
 * reactions existed reads back as.
 */
const val DEFAULT_REACTION = "❤️"

/**
 * The row offered on long-press, Telegram-style. Kept in sync with the Vue
 * frontend's QUICK_REACTIONS (frontend/src/lib/emoji.js).
 */
val QUICK_REACTIONS = listOf("❤️", "👍", "👎", "🔥", "🎉", "😁", "😢", "🤔")

/**
 * The wider set behind the picker's "more" button. The node accepts any
 * emoji, so this is only about what the phone offers without a keyboard.
 */
val MORE_REACTIONS = QUICK_REACTIONS + listOf(
    "😍", "🤩", "😂", "🤣", "😭", "😱", "🤯", "🥰",
    "😎", "🙏", "👏", "💯", "⚡", "🎂", "🍾", "🥳",
    "🤡", "💩", "🥱", "🤨", "😴", "🤝", "👀", "🫡",
)

/**
 * One chip per emoji somebody put on the tweet, ordered by popularity with
 * the emoji itself breaking ties so the row doesn't reshuffle between
 * refreshes. Tapping a chip reacts with it; tapping the one you already
 * hold takes your reaction back.
 */
@Composable
fun ReactionChips(
    status: Tweet,
    onReact: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val chips = remember(status.reactions) {
        status.reactions.entries
            .filter { it.value > 0 }
            .sortedWith(compareByDescending<Map.Entry<String, Long>> { it.value }.thenBy { it.key })
    }
    if (chips.isEmpty()) return

    FlowRow(modifier = modifier) {
        chips.forEach { (emoji, count) ->
            val mine = emoji == status.myReaction
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .padding(end = 4.dp, bottom = 4.dp)
                    .clip(CircleShape)
                    .background(if (mine) colorScheme.primaryContainer else colorScheme.surfaceVariant)
                    .border(
                        width = 1.dp,
                        color = if (mine) colorScheme.primary else colorScheme.outlineVariant,
                        shape = CircleShape,
                    )
                    .clickable { onReact(emoji) }
                    .padding(horizontal = 8.dp, vertical = 2.dp)
                    .semantics {
                        contentDescription = "$count reacted with $emoji"
                    },
            ) {
                Text(text = emoji, fontSize = 14.sp)
                Text(
                    text = formatNumber(count, 1000),
                    color = if (mine) colorScheme.primary else warpdroidColors.tertiaryTextColor,
                    style = LocalPreferences.current.statusTextStyles.medium,
                    modifier = Modifier.padding(start = 4.dp),
                )
            }
        }
    }
}

/**
 * The emoji row a long press on the reaction button opens, anchored under
 * it the way the retweet menu is. "More" swaps the row for the wider set
 * rather than opening a second surface.
 */
@Composable
fun ReactionPickerMenu(
    expanded: Boolean,
    selected: String,
    onDismiss: () -> Unit,
    onSelect: (String) -> Unit,
) {
    // Reset back to the short row every time the menu reopens.
    var showAll by remember(expanded) { mutableStateOf(false) }

    DropdownMenu(expanded = expanded, onDismissRequest = onDismiss) {
        FlowRow(
            modifier = Modifier
                .widthIn(max = 260.dp)
                .padding(horizontal = 8.dp),
            verticalArrangement = Arrangement.Center,
        ) {
            (if (showAll) MORE_REACTIONS else QUICK_REACTIONS).forEach { emoji ->
                Text(
                    text = emoji,
                    fontSize = 22.sp,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .padding(2.dp)
                        .clip(CircleShape)
                        .background(if (emoji == selected) colorScheme.primaryContainer else colorScheme.surface)
                        .clickable { onSelect(emoji) }
                        .padding(6.dp),
                )
            }
            if (!showAll) {
                IconButton(onClick = { showAll = true }) {
                    Icon(
                        painter = painterResource(R.drawable.ic_add_24dp),
                        tint = warpdroidColors.tertiaryTextColor,
                        contentDescription = stringResource(R.string.action_more_reactions),
                    )
                }
            }
        }
    }
}
