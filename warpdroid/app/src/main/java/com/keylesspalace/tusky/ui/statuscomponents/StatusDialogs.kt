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
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.input.TextFieldLineLimits
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.foundation.text.input.setTextAndPlaceCursorAtEnd
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuAnchorType
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.integerArrayResource
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.ui.TuskyTextButton

@Composable
internal fun rememberDialogState(): DialogState = remember { DialogState() }

internal data class DialogState(
    internal var dialogData: MutableState<(@Composable () -> Unit)?> = mutableStateOf(null)
) {
    fun show(
        dialog: @Composable () -> Unit
    ) {
        dialogData.value = dialog
    }
    fun hide() {
        dialogData.value = null
    }
}

@Composable
internal fun Dialog(
    state: DialogState
) {
    state.dialogData.value?.invoke()
}

internal fun DialogState.showDeleteStatusDialog(
    onDelete: () -> Unit
) {
    show {
        ConfirmationDialog(
            message = stringResource(R.string.dialog_delete_post_warning),
            onConfirm = onDelete
        )
    }
}

internal fun DialogState.showConfirmRedraftDialog(
    onRedraft: () -> Unit
) {
    show {
        ConfirmationDialog(
            message = stringResource(R.string.dialog_redraft_post_warning),
            onConfirm = onRedraft
        )
    }
}

internal fun DialogState.showBlockAccountDialog(
    accountUsername: String,
    onBlock: () -> Unit
) {
    show {
        ConfirmationDialog(
            message = stringResource(R.string.dialog_block_warning, accountUsername),
            onConfirm = onBlock
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
internal fun DialogState.showConfirmMuteDialog(
    accountUsername: String,
    onMute: (notifications: Boolean, duration: Int?) -> Unit
) {
    show {
        var hideNotifications by remember { mutableStateOf(false) }
        val durationLabels = stringArrayResource(R.array.mute_duration_names)
        val durationValues = integerArrayResource(R.array.mute_duration_values)
        var durationMenuExpanded by remember { mutableStateOf(false) }
        val durationTextFieldState = rememberTextFieldState(durationLabels[0])
        var selectedDuration: Int by remember { mutableIntStateOf(0) }

        AlertDialog(
            text = {
                Column(
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(stringResource(R.string.dialog_mute_warning, accountUsername))

                    Row(
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Checkbox(
                            checked = hideNotifications,
                            onCheckedChange = {
                                hideNotifications = !hideNotifications
                            }
                        )
                        Spacer(modifier = Modifier.width(4.dp))
                        Text(stringResource(R.string.dialog_mute_hide_notifications))
                    }

                    ExposedDropdownMenuBox(
                        expanded = durationMenuExpanded,
                        onExpandedChange = { durationMenuExpanded = it }
                    ) {
                        OutlinedTextField(
                            modifier = Modifier.menuAnchor(ExposedDropdownMenuAnchorType.PrimaryNotEditable),
                            state = durationTextFieldState,
                            readOnly = true,
                            lineLimits = TextFieldLineLimits.SingleLine,
                            label = { Text(stringResource(R.string.label_duration)) },
                            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = durationMenuExpanded) },
                        )
                        ExposedDropdownMenu(
                            expanded = durationMenuExpanded,
                            onDismissRequest = { durationMenuExpanded = false },
                            containerColor = colorScheme.surface,
                        ) {
                            durationLabels.forEachIndexed { index, option ->
                                DropdownMenuItem(
                                    text = { Text(option, style = MaterialTheme.typography.bodyLarge) },
                                    onClick = {
                                        durationTextFieldState.setTextAndPlaceCursorAtEnd(option)
                                        selectedDuration = index
                                        durationMenuExpanded = false
                                    },
                                    modifier = if (index == selectedDuration) {
                                        Modifier.background(colorScheme.primary.copy(alpha = 0.25f))
                                    } else {
                                        Modifier
                                    }
                                )
                            }
                        }
                    }
                }
            },
            onDismissRequest = {
                hide()
            },
            confirmButton = {
                TuskyTextButton(
                    text = stringResource(android.R.string.ok),
                    onClick = {
                        hide()
                        // workaround to make indefinite muting work with Mastodon 3.3.0
                        // https://github.com/tuskyapp/Tusky/issues/2107
                        val duration = if (selectedDuration == 0) {
                            null
                        } else {
                            durationValues[selectedDuration]
                        }

                        onMute(hideNotifications, duration)
                    }
                )
            },
            dismissButton = {
                TuskyTextButton(
                    text = stringResource(android.R.string.cancel),
                    onClick = {
                        hide()
                    }
                )
            },
            shape = RoundedCornerShape(16.dp),
            containerColor = colorScheme.background,
            tonalElevation = 6.dp
        )
    }
}

internal fun DialogState.showRemovePostDialog(
    username: String,
    onRemove: () -> Unit
) {
    show {
        ConfirmationDialog(
            message = stringResource(R.string.remove_post_warning, username),
            confirmAction = stringResource(R.string.remove_post),
            onConfirm = onRemove
        )
    }
}

@Composable
private fun DialogState.ConfirmationDialog(
    message: String,
    confirmAction: String = stringResource(android.R.string.ok),
    onConfirm: () -> Unit
) {
    AlertDialog(
        text = { Text(message) },
        onDismissRequest = {
            hide()
        },
        confirmButton = {
            TuskyTextButton(
                text = confirmAction,
                onClick = {
                    onConfirm()
                    hide()
                }
            )
        },
        dismissButton = {
            TuskyTextButton(
                text = stringResource(android.R.string.cancel),
                onClick = {
                    hide()
                }
            )
        },
        shape = RoundedCornerShape(16.dp),
        containerColor = colorScheme.background,
        tonalElevation = 6.dp
    )
}
