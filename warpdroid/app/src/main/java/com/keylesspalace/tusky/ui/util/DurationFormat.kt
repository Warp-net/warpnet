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

package com.keylesspalace.tusky.ui.util

import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.res.pluralStringResource
import com.keylesspalace.tusky.R
import java.text.NumberFormat

private val numberFormat = NumberFormat.getNumberInstance()

@Composable
@ReadOnlyComposable
fun Int.formatDuration() = buildString {
    val days = div(SECONDS_PER_DAY)
    val hours = mod(SECONDS_PER_DAY).div(SECONDS_PER_HOUR)
    val minutes = mod(SECONDS_PER_DAY).mod(SECONDS_PER_HOUR).div(SECONDS_PER_MINUTE)
    val seconds = mod(SECONDS_PER_DAY).mod(SECONDS_PER_HOUR).mod(SECONDS_PER_MINUTE)

    if (days != 0) {
        append(pluralStringResource(R.plurals.days, days, numberFormat.format(days)))
        append(" ")
    }
    if (hours != 0) {
        append(pluralStringResource(R.plurals.hours, hours, numberFormat.format(hours)))
        append(" ")
    }
    if (minutes != 0) {
        append(pluralStringResource(R.plurals.minutes, minutes, numberFormat.format(minutes)))
        append(" ")
    }
    if (seconds != 0) {
        append(pluralStringResource(R.plurals.seconds, seconds, numberFormat.format(seconds)))
    }
}.trim()

private const val SECONDS_PER_MINUTE = 60
private const val SECONDS_PER_HOUR = 60 * 60
private const val SECONDS_PER_DAY = 60 * 60 * 24
