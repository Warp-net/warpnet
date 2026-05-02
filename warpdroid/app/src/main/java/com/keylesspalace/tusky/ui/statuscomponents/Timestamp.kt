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

import android.content.Context
import android.text.format.DateUtils
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.util.AbsoluteTimeFormatter
import com.keylesspalace.tusky.util.getRelativeTimeSpanString
import java.util.Date
import kotlin.time.Duration.Companion.minutes
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private val absoluteTimeFormatter = AbsoluteTimeFormatter()

private val UPDATE_INTERVAL = 1.minutes

@Composable
fun Timestamp(
    date: Date,
    isEdited: Boolean,
    textColor: Color,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val useAbsoluteTime = LocalPreferences.current.useAbsoluteTime
    var timestamp by remember(date, isEdited, useAbsoluteTime) {
        mutableStateOf(
            createFormattedTimestamp(date = date, context = context, isEdited = isEdited, useAbsoluteTime = useAbsoluteTime)
        )
    }

    if (!useAbsoluteTime) {
        // periodically refresh the time
        val coroutineScope: CoroutineScope = rememberCoroutineScope()

        LifecycleResumeEffect(Unit) {
            val updateJob = coroutineScope.launch {
                while (true) {
                    timestamp = createFormattedTimestamp(date, context, isEdited, false)
                    delay(UPDATE_INTERVAL)
                }
            }
            onPauseOrDispose {
                updateJob.cancel()
            }
        }
    }

    val description = buildString {
        if (useAbsoluteTime) {
            append(timestamp)
        } else {
            append(
                DateUtils.getRelativeTimeSpanString(
                    date.time,
                    System.currentTimeMillis(),
                    DateUtils.SECOND_IN_MILLIS,
                    DateUtils.FORMAT_ABBREV_RELATIVE
                )
            )
        }
        if (isEdited) {
            append(", ")
            append(stringResource(R.string.description_post_edited))
        }
    }

    Text(
        text = timestamp,
        color = textColor,
        style = LocalPreferences.current.statusTextStyles.medium,
        maxLines = 1,
        modifier = modifier.semantics {
            contentDescription = description
        }
    )
}

private fun createFormattedTimestamp(
    date: Date,
    context: Context,
    isEdited: Boolean,
    useAbsoluteTime: Boolean
): String {
    val formattedTimestamp = if (useAbsoluteTime) {
        absoluteTimeFormatter.format(date, true)
    } else {
        val then: Long = date.time
        val now = System.currentTimeMillis()
        getRelativeTimeSpanString(context, then, now)
    }
    return if (isEdited) {
        "$formattedTimestamp*"
    } else {
        formattedTimestamp
    }
}
