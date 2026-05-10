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
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import coil3.compose.AsyncImage
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.ui.warpdroidDefaultCornerShape
import site.warpnet.warpdroid.ui.warpdroidDefaultRadius

@Composable
fun Avatar(
    url: String,
    staticUrl: String,
    isBot: Boolean,
    retweetedAvatarUrl: String?,
    staticRetweetedAvatarUrl: String?,
    onOpenProfile: (() -> Unit)?,
    modifier: Modifier = Modifier
) {
    Box(
        modifier
            .size(48.dp)
            .semantics { hideFromAccessibility() }
    ) {
        val animateAvatars = LocalPreferences.current.animateAvatars
        val placeholder = painterResource(R.drawable.avatar_default)
        AsyncImage(
            model = if (animateAvatars) {
                url
            } else {
                staticUrl
            },
            contentDescription = null,
            placeholder = placeholder,
            error = placeholder,
            modifier = Modifier
                .run {
                    if (retweetedAvatarUrl == null) {
                        fillMaxSize()
                    } else {
                        fillMaxSize(0.75f)
                    }
                }
                .align(Alignment.TopStart)
                .clip(
                    if (retweetedAvatarUrl == null) {
                        warpdroidDefaultCornerShape
                    } else {
                        RoundedCornerShape(warpdroidDefaultRadius * 0.75f)
                    }
                )
                .run {
                    if (onOpenProfile != null) {
                        clickable { onOpenProfile() }
                    } else {
                        this
                    }
                }
        )
        if (retweetedAvatarUrl != null) {
            val retweetedAvatarPlaceholder = painterResource(R.drawable.avatar_default)
            AsyncImage(
                model = if (animateAvatars) {
                    retweetedAvatarUrl
                } else {
                    staticRetweetedAvatarUrl
                },
                contentDescription = null,
                placeholder = retweetedAvatarPlaceholder,
                error = retweetedAvatarPlaceholder,
                modifier = Modifier
                    .fillMaxSize(0.5f)
                    .align(Alignment.BottomEnd)
                    .clip(
                        RoundedCornerShape(warpdroidDefaultRadius / 2)
                    )
            )
        } else if (isBot && LocalPreferences.current.showBotBadge) {
            Icon(
                painterResource(R.drawable.ic_bot_24dp),
                tint = warpdroidColors.primaryTextColor,
                contentDescription = null,
                modifier = Modifier
                    .fillMaxSize(0.5f)
                    .align(Alignment.BottomEnd)
                    .clip(RoundedCornerShape(warpdroidDefaultRadius))
                    .background(warpdroidColors.windowBackground.copy(alpha = 0.75f))
                    .padding(2.dp)
            )
        }
    }
}
