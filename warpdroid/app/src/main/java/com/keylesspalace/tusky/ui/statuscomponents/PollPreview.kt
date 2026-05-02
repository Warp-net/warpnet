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

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.entity.NewPoll
import com.keylesspalace.tusky.ui.TuskyTheme
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.ui.tuskyDefaultCornerShape
import com.keylesspalace.tusky.ui.util.formatDuration

@Composable
fun PollPreview(
    poll: NewPoll,
    modifier: Modifier
) {
    OutlinedCard(
        shape = tuskyDefaultCornerShape,
        modifier = modifier
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_insert_chart_24dp),
                    tint = tuskyColors.secondaryTextColor,
                    contentDescription = null
                )
                Text(
                    text = stringResource(R.string.poll),
                    fontWeight = FontWeight.Bold,
                    color = tuskyColors.secondaryTextColor
                )
            }

            Spacer(Modifier.height(4.dp))

            poll.options.forEach { option ->
                PollOption(option, poll.multiple)
            }

            Text(
                text = poll.expiresIn.formatDuration(),
                color = tuskyColors.secondaryTextColor
            )
        }
    }
}

@Composable
private fun PollOption(option: String, multiple: Boolean) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        Icon(
            painter = if (multiple) {
                painterResource(R.drawable.ic_check_box_outline_blank_18dp)
            } else {
                painterResource(R.drawable.ic_radio_button_unchecked_18dp)
            },
            tint = tuskyColors.secondaryTextColor,
            contentDescription = null
        )
        Text(
            text = option,
            color = tuskyColors.secondaryTextColor

        )
    }
    Spacer(Modifier.height(4.dp))
}

@PreviewLightDark
@Composable
fun PollPreviewPreview() {
    TuskyTheme {
        PollPreview(
            poll = NewPoll(
                options = listOf(
                    "Yes",
                    "No",
                    "Maybe"
                ),
                expiresIn = 21600,
                multiple = false
            ),
            modifier = Modifier.padding(16.dp)
        )
    }
}
