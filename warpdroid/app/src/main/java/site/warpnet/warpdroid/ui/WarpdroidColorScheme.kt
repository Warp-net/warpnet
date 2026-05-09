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

import android.annotation.SuppressLint
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

data class WarpdroidColorScheme(
    val primaryTextColor: Color,
    val secondaryTextColor: Color,
    val tertiaryTextColor: Color,
    val disabledTextColor: Color,
    val backgroundAccent: Color,
    val windowBackground: Color,
    val likeButtonActiveColor: Color,
    val bookmarkButtonActiveColor: Color,
    val placeholderColor: Color
)

@SuppressLint("CompositionLocalNaming")
val WarpdroidColors = staticCompositionLocalOf {
    WarpdroidColorScheme(
        primaryTextColor = Color.Unspecified,
        secondaryTextColor = Color.Unspecified,
        tertiaryTextColor = Color.Unspecified,
        disabledTextColor = Color.Unspecified,
        backgroundAccent = Color.Unspecified,
        windowBackground = Color.Unspecified,
        likeButtonActiveColor = Color.Unspecified,
        bookmarkButtonActiveColor = Color.Unspecified,
        placeholderColor = Color.Unspecified
    )
}

val warpdroidColors: WarpdroidColorScheme
    @Composable
    get() = WarpdroidColors.current
