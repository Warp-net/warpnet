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
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import androidx.constraintlayout.compose.ChainStyle
import androidx.constraintlayout.compose.ConstraintLayout
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.ui.TuskyPreviewTheme
import com.keylesspalace.tusky.ui.tuskyColors

@Composable
fun StatusPlaceholder(
    modifier: Modifier = Modifier,
    showStatusInfo: Boolean = false
) {
    Column(
        modifier
            .semantics {
                hideFromAccessibility()
            }
            .padding(top = 8.dp)
    ) {
        if (showStatusInfo) {
            PlaceholderBox(
                modifier = Modifier
                    .padding(start = 14.dp, end = 14.dp, bottom = 10.dp)
                    .fillMaxWidth()
                    .height(24.dp)
            )
        }
        Row(
            modifier = Modifier.padding(start = 14.dp, top = 6.dp, end = 14.dp)
        ) {
            PlaceholderBox(
                modifier = Modifier.size(48.dp)
            )

            Spacer(modifier = Modifier.width(16.dp))

            Column(
                modifier = Modifier.weight(1f)
            ) {
                PlaceholderBox(
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(24.dp),
                )

                PlaceholderBox(
                    modifier = Modifier
                        .padding(top = 16.dp)
                        .fillMaxWidth()
                        .height(64.dp),
                )
            }
        }
        PlaceholderButtons(
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 72.dp, end = 8.dp, top = 2.dp)
        )
        HorizontalDivider()
    }
}

@Composable
private fun PlaceholderBox(
    modifier: Modifier = Modifier
) {
    Box(
        modifier.background(tuskyColors.placeholderColor, RoundedCornerShape(6.dp))
    )
}

@Composable
private fun PlaceholderButtons(
    modifier: Modifier = Modifier
) {
    ConstraintLayout(
        modifier = modifier
            .heightIn(min = 36.dp)
    ) {
        val (replyButton, reblogButton, favButton, bookmarkButton, moreButton) = createRefs()

        createHorizontalChain(replyButton, reblogButton, favButton, bookmarkButton, moreButton, chainStyle = ChainStyle.SpreadInside)

        Icon(
            painter = painterResource(R.drawable.ic_reply_24dp),
            tint = tuskyColors.placeholderColor,
            contentDescription = null,
            modifier = Modifier.constrainAs(replyButton) {
                start.linkTo(parent.start)
                end.linkTo(reblogButton.start)
                centerVerticallyTo(parent)
            }
        )

        Icon(
            painter = painterResource(R.drawable.ic_repeat_24dp),
            tint = tuskyColors.placeholderColor,
            contentDescription = null,
            modifier = Modifier
                .size(24.dp)
                .constrainAs(reblogButton) {
                    start.linkTo(replyButton.end)
                    end.linkTo(favButton.start)
                    centerVerticallyTo(parent)
                }
        )

        Icon(
            painter = painterResource(R.drawable.ic_star_24dp),
            tint = tuskyColors.placeholderColor,
            contentDescription = null,
            modifier = Modifier
                .size(24.dp)
                .constrainAs(favButton) {
                    start.linkTo(reblogButton.end)
                    end.linkTo(bookmarkButton.start)
                    centerVerticallyTo(parent)
                }
        )

        Icon(
            painter = painterResource(R.drawable.ic_bookmark_24dp),
            tint = tuskyColors.placeholderColor,
            contentDescription = null,
            modifier = Modifier
                .size(24.dp)
                .constrainAs(bookmarkButton) {
                    start.linkTo(favButton.end)
                    end.linkTo(moreButton.start)
                    centerVerticallyTo(parent)
                }
        )

        Icon(
            painter = painterResource(R.drawable.ic_more_horiz_24dp),
            tint = tuskyColors.placeholderColor,
            contentDescription = null,
            modifier = Modifier.constrainAs(moreButton) {
                start.linkTo(bookmarkButton.end)
                end.linkTo(parent.end)
                centerVerticallyTo(parent)
            }
        )
    }
}

@PreviewLightDark
@Composable
fun StatusPlaceholderPreview() {
    TuskyPreviewTheme {
        Box(modifier = Modifier.background(color = colorScheme.background)) {
            StatusPlaceholder(showStatusInfo = true)
        }
    }
}
