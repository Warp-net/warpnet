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

package site.warpnet.warpdroid.ui

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

sealed class WarpdroidButtonSize(
    val padding: PaddingValues,
    val minHeight: Dp
) {
    @Stable
    data object Small : WarpdroidButtonSize(
        padding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
        minHeight = 32.dp
    )

    @Stable
    data object Standard : WarpdroidButtonSize(
        padding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
        minHeight = 40.dp
    )
}

@Composable
fun WarpdroidButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: WarpdroidButtonSize = WarpdroidButtonSize.Standard
) {
    Button(
        onClick = onClick,
        shape = warpdroidDefaultCornerShape,
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
fun WarpdroidOutlinedButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: WarpdroidButtonSize = WarpdroidButtonSize.Standard
) {
    OutlinedButton(
        onClick = onClick,
        shape = warpdroidDefaultCornerShape,
        border = BorderStroke(
            width = 1.dp,
            color = warpdroidColors.backgroundAccent,
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
fun WarpdroidTextButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: WarpdroidButtonSize = WarpdroidButtonSize.Standard
) {
    TextButton(
        onClick = onClick,
        shape = warpdroidDefaultCornerShape,
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
    WarpdroidPreviewTheme {
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
                WarpdroidButton(
                    text = "Edit",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                )
                WarpdroidButton(
                    text = "Edit",
                    onClick = {},
                    size = WarpdroidButtonSize.Small,
                    modifier = Modifier.padding(4.dp),
                )
            }
            Row(
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth()
            ) {
                WarpdroidOutlinedButton(
                    text = "Delete",
                    onClick = {},
                    modifier = Modifier.padding(4.dp)
                )
                WarpdroidOutlinedButton(
                    text = "Delete",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                    size = WarpdroidButtonSize.Small
                )
            }
            Row(
                horizontalArrangement = Arrangement.SpaceEvenly,
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth()
            ) {
                WarpdroidTextButton(
                    text = "More",
                    onClick = {},
                    modifier = Modifier.padding(4.dp),
                )
                WarpdroidTextButton(
                    text = "More",
                    onClick = {},
                    size = WarpdroidButtonSize.Small,
                    modifier = Modifier.padding(4.dp),
                )
            }
        }
    }
}
