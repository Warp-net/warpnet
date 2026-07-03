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
import site.warpnet.warpdroid.settings.AppTheme
import site.warpnet.warpdroid.ui.preferences.LocalPreferences
import site.warpnet.warpdroid.ui.preferences.PreferencesProvider

// Warpnet brand magenta (canonical from the frontend). Names kept historically.
val warpdroidBlueLight = Color(0xFFF4D5E1)
val warpdroidBlueDark = Color(0xFFC5007F)
val warpdroidOrange = Color(0xFFCA8F04)
val warpdroidOrangeLight = Color(0xFFFAB207)
val warpdroidGreenDark = Color(0xFF00731B)
val warpdroidGreen = Color(0xFF19A341)
val warpdroidGreenLight = Color(0xFF25D069)

val warpdroidGreyBlueLight = Color(0xFFF4D5E1)
val warpdroidGreyBlueDark = Color(0xFF630241)

val warpdroidGrey10 = Color(0xFF16191F)
val warpdroidGrey20 = Color(0xFF282C37)
val warpdroidGrey30 = Color(0xFF444B5D)
val warpdroidGrey40 = Color(0xFF596378)
val warpdroidGrey50 = Color(0xFF6E7B92)
val warpdroidGrey70 = Color(0xFF9BAEC8)
val warpdroidGrey80 = Color(0xFFB9C8D8)
val warpdroidGrey90 = Color(0xFFD9E1E8)
val warpdroidGrey95 = Color(0xFFEBEFF4)

val warpdroidDefaultRadius: Dp = 8.dp
val warpdroidDefaultCornerShape = RoundedCornerShape(warpdroidDefaultRadius)

private val LightColorScheme = lightColorScheme(
    primary = warpdroidBlueDark,
    onPrimary = Color.White,
    inversePrimary = warpdroidBlueLight,
    secondary = warpdroidBlueDark,
    onSecondary = Color.White,
    surface = warpdroidGrey95,
    background = Color.White,
    surfaceContainer = warpdroidGrey95,
    surfaceContainerLowest = warpdroidGrey95,
    surfaceContainerLow = warpdroidGrey95,
    surfaceContainerHigh = warpdroidGrey95,
    surfaceContainerHighest = warpdroidGrey95,
    surfaceVariant = warpdroidGrey95,
    secondaryContainer = warpdroidGreyBlueLight,
    outline = warpdroidGrey50,
    outlineVariant = warpdroidGrey70,
)

private val LightWarpdroidColorScheme = WarpdroidColorScheme(
    primaryTextColor = warpdroidGrey10,
    secondaryTextColor = warpdroidGrey20,
    tertiaryTextColor = warpdroidGrey30,
    disabledTextColor = warpdroidGrey70,
    backgroundAccent = warpdroidGrey70,
    windowBackground = warpdroidGrey80,
    likeButtonActiveColor = warpdroidOrange,
    bookmarkButtonActiveColor = warpdroidGreenDark,
    placeholderColor = warpdroidGrey90
)

private val DarkColorScheme = darkColorScheme(
    primary = warpdroidBlueDark,
    onPrimary = Color.White,
    inversePrimary = warpdroidBlueLight,
    secondary = warpdroidBlueDark,
    onSecondary = Color.White,
    surface = warpdroidGrey30,
    background = warpdroidGrey20,
    surfaceContainer = warpdroidGrey30,
    surfaceContainerLowest = warpdroidGrey30,
    surfaceContainerLow = warpdroidGrey30,
    surfaceContainerHigh = warpdroidGrey30,
    surfaceContainerHighest = warpdroidGrey30,
    surfaceVariant = warpdroidGrey30,
    secondaryContainer = warpdroidGreyBlueDark,
    outline = warpdroidGrey70,
    outlineVariant = warpdroidGrey40
)

private val DarkWarpdroidColorScheme = WarpdroidColorScheme(
    primaryTextColor = Color.White,
    secondaryTextColor = warpdroidGrey90,
    tertiaryTextColor = warpdroidGrey70,
    disabledTextColor = warpdroidGrey40,
    backgroundAccent = warpdroidGrey40,
    windowBackground = warpdroidGrey10,
    likeButtonActiveColor = warpdroidOrangeLight,
    bookmarkButtonActiveColor = warpdroidGreen,
    placeholderColor = warpdroidGrey40
)

private val BlackColorScheme = DarkColorScheme.copy(
    background = Color.Black,
    surface = warpdroidGrey10,
    surfaceContainer = warpdroidGrey10,
    surfaceContainerLowest = warpdroidGrey10,
    surfaceContainerLow = warpdroidGrey10,
    surfaceContainerHigh = warpdroidGrey10,
    surfaceContainerHighest = warpdroidGrey10,
    surfaceVariant = warpdroidGrey10
)

private val BlackWarpdroidColorScheme = DarkWarpdroidColorScheme.copy(
    windowBackground = Color.Black,
    placeholderColor = warpdroidGrey30
)

private val WarpdroidTypography = Typography(
    bodyLarge = TextStyle(fontSize = 14.sp, lineHeight = 18.sp),
    bodyMedium = TextStyle(fontSize = 16.sp, lineHeight = 20.sp),
    bodySmall = TextStyle(fontSize = 18.sp, lineHeight = 24.sp)
)

@Composable
fun WarpdroidTheme(
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
        val warpdroidColors = if (!useDarkTheme) {
            LightWarpdroidColorScheme
        } else {
            if (LocalPreferences.current.theme == AppTheme.BLACK || LocalPreferences.current.theme == AppTheme.AUTO_SYSTEM_BLACK) {
                BlackWarpdroidColorScheme
            } else {
                DarkWarpdroidColorScheme
            }
        }
        CompositionLocalProvider(LocalMinimumInteractiveComponentSize provides Dp.Unspecified) {
            CompositionLocalProvider(WarpdroidColors provides warpdroidColors) {
                MaterialTheme(
                    colorScheme = colors,
                    typography = WarpdroidTypography,
                    content = content
                )
            }
        }
    }
}

// for use in Previews, doesn't support the black theme or provide LocalPreferences
@Composable
fun WarpdroidPreviewTheme(
    useDarkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit
) {
    val colors = if (!useDarkTheme) {
        LightColorScheme
    } else {
        DarkColorScheme
    }
    val warpdroidColors = if (!useDarkTheme) {
        LightWarpdroidColorScheme
    } else {
        DarkWarpdroidColorScheme
    }
    CompositionLocalProvider(LocalMinimumInteractiveComponentSize provides Dp.Unspecified) {
        CompositionLocalProvider(WarpdroidColors provides warpdroidColors) {
            MaterialTheme(
                colorScheme = colors,
                typography = WarpdroidTypography,
                content = content
            )
        }
    }
}
