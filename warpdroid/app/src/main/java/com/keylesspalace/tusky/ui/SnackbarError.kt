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

import androidx.annotation.StringRes
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.fragment.app.Fragment
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.google.android.material.snackbar.Snackbar
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.viewmodel.StatusActionsViewModel

sealed class SnackbarError {

    @Composable
    abstract fun message(): String

    abstract val retryAction: (() -> Unit)?

    data class StringMessage(
        val message: String,
        override val retryAction: (() -> Unit)? = null
    ) : SnackbarError() {
        @Composable
        override fun message() = message
    }

    data class ResourceMessage(
        @StringRes val message: Int,
        override val retryAction: (() -> Unit)? = null
    ) : SnackbarError() {
        @Composable
        override fun message() = stringResource(message)
    }
}

@Composable
fun Fragment.ErrorSnackbars(
    viewModel: StatusActionsViewModel,
    legacyFallback: Boolean,
    modifier: Modifier = Modifier
) {
    val snackbarHostState = remember { SnackbarHostState() }

    SnackbarHost(
        hostState = snackbarHostState,
        modifier = modifier
    )

    val error by viewModel.snackbarErrors.collectAsStateWithLifecycle(initialValue = null)

    val snackbarMessage = error?.message()
    val snackbarActionLabel = error?.retryAction?.let {
        stringResource(R.string.action_retry)
    }

    LaunchedEffect(error) {
        if (snackbarMessage != null) {
            if (legacyFallback) {
                view?.let {
                    val snackbar = Snackbar.make(it, snackbarMessage, Snackbar.LENGTH_INDEFINITE)
                    if (snackbarActionLabel != null) {
                        snackbar.setAction(snackbarMessage) {
                            error?.retryAction?.invoke()
                        }
                    }
                    snackbar.show()
                }
            } else {
                val snackbarResult = snackbarHostState
                    .showSnackbar(
                        message = snackbarMessage,
                        actionLabel = snackbarActionLabel,
                        withDismissAction = true,
                        duration = SnackbarDuration.Long
                    )
                if (error?.retryAction != null && snackbarResult == SnackbarResult.ActionPerformed) {
                    error?.retryAction?.invoke()
                }
                viewModel.snackbarErrorShown()
            }
        }
    }
}
