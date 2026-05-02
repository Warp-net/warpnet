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

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.LocalMinimumInteractiveComponentSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.settings.AppTheme
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.preferences.PreferencesProvider

val tuskyBlueLight = Color(0xFF3C9ADD)
val tuskyBlueDark = Color(0xFF217ABA)
val tuskyBlueLighter = Color(0xFF56A7E1)
val tuskyOrange = Color(0xFFCA8F04)
val tuskyOrangeLight = Color(0xFFFAB207)
val tuskyGreenDark = Color(0xFF00731B)
val tuskyGreen = Color(0xFF19A341)
val tuskyGreenLight = Color(0xFF25D069)
val tuskyGreenLighter = Color(0xFFCCFFD8)
val tuskyRed = Color(0xFFDF1553)
val tuskyRedLighter = Color(0xFFFF7287)

val tuskyGreyBlueLight = Color(0xFF94BADA)
val tuskyGreyBlueDark = Color(0xFF305879)

val tuskyGrey05 = Color(0xFF070B14)
val tuskyGrey10 = Color(0xFF16191F)
val tuskyGrey15 = Color(0xFF21222C)
val tuskyGrey20 = Color(0xFF282C37)
val tuskyGrey25 = Color(0xFF313543)
val tuskyGrey30 = Color(0xFF444B5D)
val tuskyGrey40 = Color(0xFF596378)
val tuskyGrey50 = Color(0xFF6E7B92)
val tuskyGrey70 = Color(0xFF9BAEC8)
val tuskyGrey80 = Color(0xFFB9C8D8)
val tuskyGrey90 = Color(0xFFD9E1E8)
val tuskyGrey95 = Color(0xFFEBEFF4)

val tuskyDefaultRadius: Dp = 8.dp
val tuskyDefaultCornerShape = RoundedCornerShape(tuskyDefaultRadius)

private val LightColorScheme = lightColorScheme(
    primary = tuskyBlueDark,
    onPrimary = Color.White,
    inversePrimary = tuskyBlueLight,
    secondary = tuskyBlueDark,
    onSecondary = Color.White,
    surface = tuskyGrey95,
    background = Color.White,
    surfaceContainer = tuskyGrey95,
    surfaceContainerLowest = tuskyGrey95,
    surfaceContainerLow = tuskyGrey95,
    surfaceContainerHigh = tuskyGrey95,
    surfaceContainerHighest = tuskyGrey95,
    surfaceVariant = tuskyGrey95,
    secondaryContainer = tuskyGreyBlueLight,
    outline = tuskyGrey50,
    outlineVariant = tuskyGrey70,
)

private val LightTuskyColorScheme = TuskyColorScheme(
    primaryTextColor = tuskyGrey10,
    secondaryTextColor = tuskyGrey20,
    tertiaryTextColor = tuskyGrey30,
    disabledTextColor = tuskyGrey70,
    backgroundAccent = tuskyGrey70,
    windowBackground = tuskyGrey80,
    favoriteButtonActiveColor = tuskyOrange,
    bookmarkButtonActiveColor = tuskyGreenDark,
    placeholderColor = tuskyGrey90
)

private val DarkColorScheme = darkColorScheme(
    primary = tuskyBlueLight,
    onPrimary = tuskyGrey10,
    inversePrimary = tuskyBlueDark,
    secondary = tuskyBlueLight,
    onSecondary = tuskyGrey90,
    surface = tuskyGrey30,
    background = tuskyGrey20,
    surfaceContainer = tuskyGrey30,
    surfaceContainerLowest = tuskyGrey30,
    surfaceContainerLow = tuskyGrey30,
    surfaceContainerHigh = tuskyGrey30,
    surfaceContainerHighest = tuskyGrey30,
    surfaceVariant = tuskyGrey30,
    secondaryContainer = tuskyGreyBlueDark,
    outline = tuskyGrey70,
    outlineVariant = tuskyGrey40
)

private val DarkTuskyColorScheme = TuskyColorScheme(
    primaryTextColor = Color.White,
    secondaryTextColor = tuskyGrey90,
    tertiaryTextColor = tuskyGrey70,
    disabledTextColor = tuskyGrey40,
    backgroundAccent = tuskyGrey40,
    windowBackground = tuskyGrey10,
    favoriteButtonActiveColor = tuskyOrangeLight,
    bookmarkButtonActiveColor = tuskyGreen,
    placeholderColor = tuskyGrey40
)

private val BlackColorScheme = DarkColorScheme.copy(
    onPrimary = Color.Black,
    background = Color.Black,
    surface = tuskyGrey10,
    surfaceContainer = tuskyGrey10,
    surfaceContainerLowest = tuskyGrey10,
    surfaceContainerLow = tuskyGrey10,
    surfaceContainerHigh = tuskyGrey10,
    surfaceContainerHighest = tuskyGrey10,
    surfaceVariant = tuskyGrey10
)

private val BlackTuskyColorScheme = DarkTuskyColorScheme.copy(
    windowBackground = Color.Black,
    placeholderColor = tuskyGrey30
)

private val TuskyTypography = Typography(
    bodyLarge = TextStyle(fontSize = 14.sp, lineHeight = 18.sp),
    bodyMedium = TextStyle(fontSize = 16.sp, lineHeight = 20.sp),
    bodySmall = TextStyle(fontSize = 18.sp, lineHeight = 24.sp)
)

@Composable
fun TuskyTheme(
    useDarkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    PreferencesProvider {
        val colors = if (!useDarkTheme) {
            LightColorScheme
        } else {
            if (LocalPreferences.current.theme == AppTheme.BLACK || LocalPreferences.current.theme == AppTheme.AUTO_SYSTEM_BLACK) {
                BlackColorScheme
            } else {
                DarkColorScheme
            }
        }
        val tuskyColors = if (!useDarkTheme) {
            LightTuskyColorScheme
        } else {
            if (LocalPreferences.current.theme == AppTheme.BLACK || LocalPreferences.current.theme == AppTheme.AUTO_SYSTEM_BLACK) {
                BlackTuskyColorScheme
            } else {
                DarkTuskyColorScheme
            }
        }
        CompositionLocalProvider(LocalMinimumInteractiveComponentSize provides Dp.Unspecified) {
            CompositionLocalProvider(TuskyColors provides tuskyColors) {
                MaterialTheme(
                    colorScheme = colors,
                    typography = TuskyTypography,
                    content = content
                )
            }
        }
    }
}

// for use in Previews, doesn't support the black theme or provide LocalPreferences
@Composable
fun TuskyPreviewTheme(
    useDarkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    val colors = if (!useDarkTheme) {
        LightColorScheme
    } else {
        DarkColorScheme
    }
    val tuskyColors = if (!useDarkTheme) {
        LightTuskyColorScheme
    } else {
        DarkTuskyColorScheme
    }
    CompositionLocalProvider(LocalMinimumInteractiveComponentSize provides Dp.Unspecified) {
        CompositionLocalProvider(TuskyColors provides tuskyColors) {
            MaterialTheme(
                colorScheme = colors,
                typography = TuskyTypography,
                content = content
            )
        }
    }
}
