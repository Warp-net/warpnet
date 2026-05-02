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

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

sealed class TuskyButtonSize(
    val padding: PaddingValues,
    val minHeight: Dp
) {
    @Stable
    data object Small : TuskyButtonSize(
        padding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
        minHeight = 32.dp
    )

    @Stable
    data object Standard : TuskyButtonSize(
        padding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
        minHeight = 40.dp
    )
}

@Composable
fun TuskyButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: TuskyButtonSize = TuskyButtonSize.Standard
) {
    Button(
        onClick = onClick,
        shape = tuskyDefaultCornerShape,
        contentPadding = size.padding,
        modifier = modifier
            .heightIn(min = size.minHeight)
    ) {
        Text(
            text = text,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
fun TuskyOutlinedButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: TuskyButtonSize = TuskyButtonSize.Standard
) {
    OutlinedButton(
        onClick = onClick,
        shape = tuskyDefaultCornerShape,
        border = BorderStroke(
            width = 1.dp,
            color = tuskyColors.backgroundAccent,
        ),
        contentPadding = size.padding,
        modifier = modifier
            .heightIn(min = size.minHeight)
    ) {
        Text(
            text = text,
            color = colorScheme.primary,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
fun TuskyTextButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: TuskyButtonSize = TuskyButtonSize.Standard
) {
    TextButton(
        onClick = onClick,
        shape = tuskyDefaultCornerShape,
        contentPadding = size.padding,
        modifier = modifier
            .heightIn(min = size.minHeight)
    ) {
        Text(
            text = text,
            textAlign = TextAlign.Center
        )
    }
}

@Composable
@PreviewLightDark
fun ButtonPreview() {
    TuskyPreviewTheme {
        Column(
            verticalArrangement = Arrangement.spacedBy(8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier
                .width(IntrinsicSize.Max)
                .background(colorScheme.background)
                .padding(8.dp)
        ) {
            Row(
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth()
            ) {
                TuskyButton(
                    text = "Edit",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                )
                TuskyButton(
                    text = "Edit",
                    onClick = {},
                    size = TuskyButtonSize.Small,
                    modifier = Modifier.padding(4.dp),
                )
            }
            Row(
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth()
            ) {
                TuskyOutlinedButton(
                    text = "Delete",
                    onClick = {},
                    modifier = Modifier.padding(4.dp)
                )
                TuskyOutlinedButton(
                    text = "Delete",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                    size = TuskyButtonSize.Small
                )
            }
            Row(
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth()
            ) {
                TuskyTextButton(
                    text = "More",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                )
                TuskyTextButton(
                    text = "More",
                    onClick = {},
                    size = TuskyButtonSize.Small,
                    modifier = Modifier.padding(4.dp),
                )
            }
        }
    }
}
