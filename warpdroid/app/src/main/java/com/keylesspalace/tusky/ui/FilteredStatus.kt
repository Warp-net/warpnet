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

package com.keylesspalace.tusky.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.R

/** Shown in timelines for posts that have a warning filter set. **/
@Composable
fun FilteredStatus(
    filterTitle: String,
    onReveal: () -> Unit,
    modifier: Modifier = Modifier
) {
    Column(
        verticalArrangement = Arrangement.Center,
        modifier = modifier.fillMaxWidth()
    ) {
        Column(
            modifier
                .fillMaxWidth()
                .clickable(
                    onClick = onReveal
                )
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = stringResource(R.string.status_filter_placeholder_label_format, filterTitle),
                fontSize = 16.sp,
                color = tuskyColors.primaryTextColor,
            )
            Text(
                text = stringResource(R.string.status_filtered_show_anyway),
                fontSize = 16.sp,
                color = colorScheme.primary,
                fontWeight = FontWeight.Bold,
            )
        }
    }
    HorizontalDivider()
}

@Preview
@Composable
fun FilteredStatusPreview() {
    TuskyPreviewTheme {
        FilteredStatus(
            filterTitle = "#HorribleHashtag",
            onReveal = { },
            modifier = Modifier.background(colorScheme.background)
        )
    }
}
