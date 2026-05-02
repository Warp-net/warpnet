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

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.PreviewLightDark
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.util.getErrorString
import java.io.IOException
import retrofit2.HttpException

enum class MessageViewMode {
    EMPTY,
    ERROR_NETWORK,
    ERROR_OTHER
}

/**
 * Shows an elephant friend image, a message and a retry button.
 * Meant to be used full screen when a error occurs that prevents showing any content.
 * @param onRetry This function will be invoked when the retry button is clicked.
 * @param error The error that occurred. Image and message will be based on the type of the error.
 */
@Composable
fun TuskyMessageView(
    onRetry: (() -> Unit),
    error: Throwable,
    modifier: Modifier = Modifier
) {
    val context = LocalContext.current
    val mode = when (error) {
        is IOException -> MessageViewMode.ERROR_NETWORK
        is HttpException -> MessageViewMode.ERROR_NETWORK
        else -> MessageViewMode.ERROR_OTHER
    }

    TuskyMessageView(
        message = error.getErrorString(context),
        mode = mode,
        onRetry = onRetry,
        modifier = modifier
    )
}

/**
 * Shows an elephant friend image, a message and optionally a retry button.
 * Meant to be used full screen when a error occurs that prevents showing any content.
 * @param message The message to show.
 * @param mode One of the three possible modes. Influences the image that is shown.
 * @param onRetry This function will be invoked when the retry button is clicked. Set to null for no retry button.
 */
@Composable
fun TuskyMessageView(
    message: String,
    mode: MessageViewMode,
    modifier: Modifier = Modifier,
    onRetry: (() -> Unit)? = null,
) {
    val image = when (mode) {
        MessageViewMode.EMPTY -> R.drawable.elephant_friend_empty
        MessageViewMode.ERROR_NETWORK -> R.drawable.errorphant_offline
        MessageViewMode.ERROR_OTHER -> R.drawable.errorphant_error
    }

    Column(
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = modifier
    ) {
        Image(
            painter = painterResource(image),
            contentDescription = null
        )
        Text(
            text = message,
            fontSize = 16.sp,
            textAlign = TextAlign.Center,
            color = tuskyColors.primaryTextColor,
            modifier = Modifier.padding(
                start = 16.dp,
                top = 16.dp,
                end = 16.dp,
                bottom = 8.dp
            )
        )
        onRetry?.let { retry ->
            TuskyOutlinedButton(
                onClick = retry,
                text = stringResource(R.string.action_retry)
            )
        }
    }
}

@PreviewLightDark
@Composable
fun TuskyMessageViewPreview() {
    TuskyPreviewTheme {
        TuskyMessageView(
            message = "An error occurred.",
            mode = MessageViewMode.ERROR_OTHER,
            onRetry = {},
            modifier = Modifier
                .background(colorScheme.background)
                .padding(16.dp)
        )
    }
}
