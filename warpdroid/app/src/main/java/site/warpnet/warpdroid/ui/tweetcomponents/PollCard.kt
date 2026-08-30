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
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import java.util.Date
import java.util.concurrent.TimeUnit
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Poll
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.viewdata.TweetViewData

/**
 * The poll attached to a tweet.
 *
 * A Warpnet vote is single-choice and final, so there are exactly two states:
 * open and unvoted, where every option is a button; and settled — voted or
 * closed — where every option is a bar showing its share. The tally stays
 * hidden until this account has had its say, matching the desktop frontend,
 * so early results can't steer later voters.
 */
@Composable
fun ColumnScope.PollCard(
    statusViewData: TweetViewData.Concrete,
    onVote: (option: Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val poll = statusViewData.actionable.poll ?: return
    if (poll.options.isEmpty()) return

    // Latch the tap so a double tap can't spend two votes on a wire where the
    // second is rejected. Reset when the poll object itself changes — that is
    // the refreshed tweet arriving with the vote applied.
    var voting by remember(poll) { mutableStateOf(false) }

    Column(modifier = modifier.fillMaxWidth()) {
        poll.options.forEachIndexed { index, option ->
            if (poll.showResults) {
                PollResultRow(
                    title = option.title,
                    percent = poll.percent(index),
                    isOwnVote = poll.votedOption == index,
                    modifier = Modifier.padding(bottom = 4.dp),
                )
            } else {
                PollChoiceRow(
                    title = option.title,
                    enabled = !voting,
                    onClick = {
                        voting = true
                        onVote(index)
                    },
                    modifier = Modifier.padding(bottom = 4.dp),
                )
            }
        }

        Text(
            text = stringResource(
                R.string.poll_info_format,
                pluralStringResource(
                    R.plurals.poll_info_votes,
                    poll.totalVotes.coerceAtMost(Int.MAX_VALUE.toLong()).toInt(),
                    poll.totalVotes.toString(),
                ),
                pollTimeLeft(poll.expiresAt),
            ),
            color = warpdroidColors.tertiaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            modifier = Modifier.padding(top = 2.dp),
        )
    }
}

/** One votable option, before this account has voted. */
@Composable
private fun PollChoiceRow(
    title: String,
    enabled: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Text(
        text = title,
        color = if (enabled) colorScheme.primary else warpdroidColors.tertiaryTextColor,
        fontWeight = FontWeight.SemiBold,
        style = LocalPreferences.current.statusTextStyles.medium,
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(percent = 50))
            .border(
                width = 1.dp,
                color = if (enabled) colorScheme.primary else colorScheme.outlineVariant,
                shape = RoundedCornerShape(percent = 50),
            )
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/** One option's share, once the poll is settled. */
@Composable
private fun PollResultRow(
    title: String,
    percent: Int,
    isOwnVote: Boolean,
    modifier: Modifier = Modifier,
) {
    val ownVoteDescription = stringResource(R.string.poll_vote)
    Box(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(4.dp))
            .border(1.dp, colorScheme.outlineVariant, RoundedCornerShape(4.dp))
            .semantics {
                if (isOwnVote) contentDescription = "$ownVoteDescription: $title, $percent%"
            },
    ) {
        // A translucent tint rather than a solid fill: the themed surface
        // shows through, so the label keeps its contrast in both themes.
        Box(modifier = Modifier.matchParentSize()) {
            Box(
                modifier = Modifier
                    .fillMaxWidth((percent / 100f).coerceIn(0f, 1f))
                    .fillMaxHeight()
                    .background(colorScheme.primary.copy(alpha = 0.25f)),
            )
        }
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Text(
                text = if (isOwnVote) "✓ $title" else title,
                color = warpdroidColors.primaryTextColor,
                fontWeight = if (isOwnVote) FontWeight.SemiBold else FontWeight.Normal,
                style = LocalPreferences.current.statusTextStyles.medium,
                modifier = Modifier.weight(1f, fill = true),
            )
            Text(
                text = "$percent%",
                color = warpdroidColors.tertiaryTextColor,
                style = LocalPreferences.current.statusTextStyles.medium,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
    }
}

/**
 * "3 days left" / "closed", matching the desktop frontend's footer. A poll
 * without a parseable deadline reads as closed — the node requires one, so
 * its absence means a payload we can't vote on anyway.
 */
@Composable
private fun pollTimeLeft(expiresAt: Date?): String {
    val remaining = expiresAt?.time?.minus(System.currentTimeMillis()) ?: 0L
    if (remaining <= 0L) return stringResource(R.string.poll_info_closed)

    val days = TimeUnit.MILLISECONDS.toDays(remaining)
    if (days > 0) return pluralStringResource(R.plurals.poll_timespan_days, days.toInt(), days.toInt())
    val hours = TimeUnit.MILLISECONDS.toHours(remaining)
    if (hours > 0) return pluralStringResource(R.plurals.poll_timespan_hours, hours.toInt(), hours.toInt())
    val minutes = TimeUnit.MILLISECONDS.toMinutes(remaining)
    if (minutes > 0) {
        return pluralStringResource(R.plurals.poll_timespan_minutes, minutes.toInt(), minutes.toInt())
    }
    val seconds = TimeUnit.MILLISECONDS.toSeconds(remaining).coerceAtLeast(1L)
    return pluralStringResource(R.plurals.poll_timespan_seconds, seconds.toInt(), seconds.toInt())
}
