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

package site.warpnet.warpdroid.components.compose.dialog

import android.content.Context
import android.text.InputFilter
import android.view.LayoutInflater
import android.widget.EditText
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.textfield.TextInputEditText
import com.google.android.material.textfield.TextInputLayout
import site.warpnet.transport.WarpnetLimits
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.DialogAddPollBinding
import site.warpnet.warpdroid.entity.NewPoll
import site.warpnet.warpdroid.util.visible

fun showAddPollDialog(
    context: Context,
    poll: NewPoll?,
    onUpdate: (NewPoll?) -> Unit,
) {
    val binding = DialogAddPollBinding.inflate(LayoutInflater.from(context))

    val fields = mutableListOf<TextInputEditText>()

    fun addChoiceField(text: String) {
        if (fields.size >= WarpnetLimits.MAX_POLL_OPTIONS) return
        val layout = TextInputLayout(context).apply {
            hint = context.getString(R.string.poll_new_choice_hint, fields.size + 1)
        }
        val field = TextInputEditText(layout.context).apply {
            filters = arrayOf<InputFilter>(InputFilter.LengthFilter(WarpnetLimits.MAX_POLL_OPTION_CHARS))
            setText(text)
        }
        layout.addView(field)
        binding.pollChoices.addView(layout)
        fields += field
        binding.addChoiceButton.visible(fields.size < WarpnetLimits.MAX_POLL_OPTIONS)
    }

    val existing = poll?.options.orEmpty()
    repeat(maxOf(WarpnetLimits.MIN_POLL_OPTIONS, existing.size)) { index ->
        addChoiceField(existing.getOrElse(index) { "" })
    }

    binding.addChoiceButton.setOnClickListener { addChoiceField("") }

    val durations = context.resources.getIntArray(R.array.poll_duration_values)
    binding.pollDurationSpinner.setSelection(
        durations.indexOf(poll?.expiresInSeconds ?: 0).takeIf { it >= 0 }
            ?: durations.indexOf(DEFAULT_POLL_DURATION_SECONDS).coerceAtLeast(0),
    )

    MaterialAlertDialogBuilder(context)
        .setTitle(R.string.create_poll_title)
        .setView(binding.root)
        .setPositiveButton(android.R.string.ok) { _, _ ->
            val options = fields.map { it.text.toString().trim() }.filter { it.isNotEmpty() }
            if (options.size < WarpnetLimits.MIN_POLL_OPTIONS) {
                onUpdate(null)
                return@setPositiveButton
            }
            onUpdate(
                NewPoll(
                    options = options,
                    expiresInSeconds = durations.getOrElse(binding.pollDurationSpinner.selectedItemPosition) {
                        DEFAULT_POLL_DURATION_SECONDS
                    },
                ),
            )
        }
        .setNegativeButton(android.R.string.cancel, null)
        .apply {
            if (poll != null) {
                setNeutralButton(R.string.action_remove) { _, _ -> onUpdate(null) }
            }
        }
        .show()
}

private const val DEFAULT_POLL_DURATION_SECONDS = 86_400
