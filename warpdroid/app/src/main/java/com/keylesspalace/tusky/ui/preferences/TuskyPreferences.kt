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

package com.keylesspalace.tusky.ui.preferences

import androidx.compose.runtime.Immutable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.Hyphens
import androidx.compose.ui.text.style.TextDirection
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.components.preference.PreferencesFragment
import com.keylesspalace.tusky.settings.AppTheme

@Immutable
data class TuskyPreferences(
    val theme: AppTheme = AppTheme.DEFAULT,

    val statusTextStyles: StatusTextStyles = StatusTextStyles(),

    val useBlurhash: Boolean = false,
    val showBotBadge: Boolean = true,
    val animateCustomEmojis: Boolean = false,
    val animateAvatars: Boolean = false,
    val useAbsoluteTime: Boolean = false,
    val showStatsInline: Boolean = false,
    val showLinkPreviews: Boolean = false,
    val readingOrder: PreferencesFragment.ReadingOrder = PreferencesFragment.ReadingOrder.NEWEST_FIRST,
    val wellbeing: WellbeingSettings = WellbeingSettings()
)

@Immutable
data class StatusTextStyles(
    val small: TextStyle = textStyle(fontSize = 14.sp),
    val medium: TextStyle = textStyle(fontSize = 16.sp),
    val large: TextStyle = textStyle(fontSize = 18.sp)
)

@Immutable
data class WellbeingSettings(
    val limitTimelineNotifications: Boolean = false,
    val hideQuantitativeStatsOnPosts: Boolean = false,
    val hideQuantitativeStatsOnProfiles: Boolean = false
)

// 1.3f = about the same as android:lineSpacingMultiplier="1.1"
fun textStyle(
    fontSize: TextUnit
) = TextStyle(
    fontSize = fontSize,
    lineHeight = fontSize * 1.3f,
    hyphens = Hyphens.Auto,
    textDirection = TextDirection.Content
)
