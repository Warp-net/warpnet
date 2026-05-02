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

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.entity.Poll
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.TuskyButtonSize
import com.keylesspalace.tusky.ui.TuskyOutlinedButton
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.statuscomponents.fake.fakeStatusViewData
import com.keylesspalace.tusky.ui.statuscomponents.fake.noopListener
import com.keylesspalace.tusky.ui.statuscomponents.fake.pollWithFourOptions
import com.keylesspalace.tusky.ui.statuscomponents.text.emojify
import com.keylesspalace.tusky.ui.statuscomponents.text.toInlineContent
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.ui.tuskyDefaultCornerShape
import com.keylesspalace.tusky.ui.tuskyGrey50
import com.keylesspalace.tusky.util.AbsoluteTimeFormatter
import com.keylesspalace.tusky.util.formatPollDuration
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewdata.calculatePercent
import java.text.NumberFormat
import java.util.Date

@Composable
fun Poll(
    statusViewData: StatusViewData.Concrete,
    isExpanded: Boolean,
    listener: StatusActionListener
) {
    val status = statusViewData.actionable
    val poll = status.poll

    if (poll == null || status.spoilerText.isNotEmpty() && !isExpanded) {
        return // no poll shown
    }

    val translatedOptions = statusViewData.translation?.data?.poll?.options

    val timestamp = System.currentTimeMillis()

    val expired = poll.expired || (poll.expiresAt != null && timestamp > poll.expiresAt.time)

    if (expired || poll.voted) {
        // no voting possible
        poll.options.forEachIndexed { index, pollOption ->
            PollOptionResult(
                title = translatedOptions?.getOrNull(index)?.title ?: pollOption.title,
                votesCount = pollOption.votesCount ?: 0,
                votersCount = poll.votersCount,
                totalVotes = poll.votesCount,
                voted = poll.ownVotes.contains(index),
                emojis = status.emojis
            )
        }
    } else {
        var choices by remember { mutableStateOf(emptyList<Int>()) }

        poll.options.forEachIndexed { index, pollOption ->
            val selected = choices.any { it == index }
            PollOption(
                title = translatedOptions?.getOrNull(index)?.title ?: pollOption.title,
                emojis = status.emojis,
                multiChoice = poll.multiple,
                selected = selected,
                onSelect = {
                    choices = if (poll.multiple) {
                        if (selected) {
                            choices - index
                        } else {
                            choices + index
                        }
                    } else {
                        listOf(index)
                    }
                }
            )
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            modifier = Modifier
                .padding(top = 6.dp)
                .height(IntrinsicSize.Max)
        ) {
            TuskyOutlinedButton(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxHeight(),
                text = stringResource(R.string.poll_vote),
                size = TuskyButtonSize.Small,
                onClick = { listener.onVoteInPoll(statusViewData, poll.id, choices) }
            )
            TuskyOutlinedButton(
                modifier = Modifier
                    .weight(1f)
                    .fillMaxHeight(),
                text = stringResource(R.string.poll_show_results),
                size = TuskyButtonSize.Small,
                onClick = { listener.onShowPollResults(statusViewData) }
            )
        }
    }
    Text(
        text = getPollInfoText(timestamp, poll),
        color = tuskyColors.tertiaryTextColor,
        style = LocalPreferences.current.statusTextStyles.medium,
        modifier = Modifier.padding(top = 6.dp)
    )
}

@Composable
private fun PollOptionResult(
    title: String,
    votesCount: Int,
    votersCount: Int?,
    totalVotes: Int,
    voted: Boolean,
    emojis: List<Emoji>
) {
    val percent = calculatePercent(votesCount, votersCount, totalVotes)

    val optionColor = if (voted) {
        tuskyGrey50 // same regardless of theme, TODO?
    } else {
        tuskyColors.backgroundAccent
    }

    val colorStops = arrayOf(
        0.0f to optionColor,
        percent.toFloat() / 100f to optionColor,
        percent.toFloat() / 100f to Color.Transparent,
        1f to Color.Transparent
    )

    val textStyle = LocalPreferences.current.statusTextStyles.medium

    Text(
        text = buildDescription(title, percent, voted, emojis),
        color = tuskyColors.primaryTextColor,
        style = textStyle,
        inlineContent = emojis.toInlineContent() + mapOf(
            CHECKMARK_CONTENT to InlineTextContent(
                placeholder = Placeholder(
                    width = textStyle.fontSize,
                    height = textStyle.fontSize,
                    placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter
                ),
                children = {
                    Icon(
                        modifier = Modifier.fillMaxSize(),
                        tint = tuskyColors.primaryTextColor,
                        painter = painterResource(R.drawable.ic_check_circle_24dp),
                        contentDescription = null
                    )
                }
            )
        ),
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 6.dp)
            .background(
                Brush.horizontalGradient(colorStops = colorStops),
                shape = RoundedCornerShape(8.dp)
            )
            .border(1.dp, optionColor, RoundedCornerShape(8.dp))
            .padding(horizontal = 12.dp, vertical = 4.dp)
    )
}

@Composable
private fun PollOption(
    title: String,
    emojis: List<Emoji>,
    multiChoice: Boolean,
    selected: Boolean,
    onSelect: () -> Unit
) {
    val optionColor = if (selected) {
        colorScheme.surface
    } else {
        tuskyColors.backgroundAccent
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 6.dp)
            .clip(tuskyDefaultCornerShape)
            .clickable {
                onSelect()
            }
            .run {
                if (selected) {
                    background(optionColor, tuskyDefaultCornerShape)
                } else {
                    this
                }
            }
            .border(1.dp, optionColor, tuskyDefaultCornerShape)
            .padding(horizontal = 12.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        if (multiChoice) {
            Checkbox(
                checked = selected,
                onCheckedChange = { onSelect() }
            )
        } else {
            RadioButton(
                selected = selected,
                onClick = onSelect
            )
        }
        Text(
            text = title.emojify(emojis),
            color = tuskyColors.primaryTextColor,
            style = LocalPreferences.current.statusTextStyles.medium,
            modifier = Modifier.padding(start = 6.dp),
            inlineContent = emojis.toInlineContent()
        )
    }
}

@Composable
private fun buildDescription(
    title: String,
    percent: Int,
    voted: Boolean,
    emojis: List<Emoji>
): AnnotatedString {
    return buildAnnotatedString {
        withStyle(SpanStyle(fontWeight = FontWeight.Bold)) {
            append(percent.toString())
            append('%')
        }
        append(' ')
        append(title.emojify(emojis))
        if (voted) {
            append(' ')
            appendInlineContent(CHECKMARK_CONTENT, "✓")
        }
    }
}

private val numberFormat = NumberFormat.getNumberInstance()
private val absoluteTimeFormatter = AbsoluteTimeFormatter()

@Composable
private fun getPollInfoText(
    timestamp: Long,
    poll: Poll
): String {
    val votesText = if (poll.votersCount == null) {
        val voters = numberFormat.format(poll.votesCount.toLong())
        pluralStringResource(R.plurals.poll_info_votes, poll.votesCount, voters)
    } else {
        val voters = numberFormat.format(poll.votersCount)
        pluralStringResource(R.plurals.poll_info_people, poll.votersCount, voters)
    }
    val pollDurationInfo = if (poll.expired) {
        stringResource(R.string.poll_info_closed)
    } else if (poll.expiresAt == null) {
        return votesText
    } else {
        if (LocalPreferences.current.useAbsoluteTime) {
            stringResource(R.string.poll_info_time_absolute, absoluteTimeFormatter.format(poll.expiresAt, false))
        } else {
            formatPollDuration(LocalContext.current, poll.expiresAt.time, timestamp)
        }
    }

    return stringResource(R.string.poll_info_format, votesText, pollDurationInfo)
}

private const val CHECKMARK_CONTENT = "check"

@PreviewLightDark
@Composable
fun ExpiredPollPreview() {
    TuskyPreviewTheme {
        Column(
            modifier = Modifier
                .background(colorScheme.background)
                .padding(8.dp)
        ) {
            Poll(
                statusViewData = fakeStatusViewData(
                    poll = pollWithFourOptions.copy(expired = true)
                ),
                isExpanded = false,
                listener = noopListener
            )
        }
    }
}

@PreviewLightDark
@Composable
fun SingleChoicePollPreview() {
    TuskyPreviewTheme {
        Column(
            modifier = Modifier
                .background(colorScheme.background)
                .padding(8.dp)
        ) {
            Poll(
                statusViewData = fakeStatusViewData(
                    poll = pollWithFourOptions.copy(
                        multiple = false,
                        expired = false,
                        expiresAt = Date(System.currentTimeMillis() + 3600000)
                    )
                ),
                isExpanded = false,
                listener = noopListener
            )
        }
    }
}

@PreviewLightDark
@Composable
fun MultiChoicePollPreview() {
    TuskyPreviewTheme {
        Column(
            modifier = Modifier
                .background(colorScheme.background)
                .padding(8.dp)
        ) {
            Poll(
                statusViewData = fakeStatusViewData(
                    poll = pollWithFourOptions.copy(
                        multiple = true,
                        expired = false,
                        expiresAt = Date(System.currentTimeMillis() + 345600000)
                    )
                ),
                isExpanded = false,
                listener = noopListener
            )
        }
    }
}
