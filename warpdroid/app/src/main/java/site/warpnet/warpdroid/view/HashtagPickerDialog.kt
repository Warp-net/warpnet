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

@file:JvmName("HashTagPickerDialog")

package site.warpnet.warpdroid.view

import android.content.Context
import android.util.Log
import android.view.KeyEvent
import android.view.LayoutInflater
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.widget.TextView
import android.widget.TextView.OnEditorActionListener
import androidx.annotation.StringRes
import androidx.appcompat.app.AlertDialog
import androidx.core.widget.doOnTextChanged
import at.connyduck.calladapter.networkresult.fold
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import site.warpnet.warpdroid.components.compose.ComposeAutoCompleteAdapter
import site.warpnet.warpdroid.components.search.SearchType
import site.warpnet.warpdroid.databinding.DialogPickHashtagBinding
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.hashtagPattern
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking

fun Context.showHashtagPickerDialog(
    api: WarpnetApi,
    @StringRes title: Int,
    onHashtagSelected: (String) -> Unit
) {
    val dialogScope = CoroutineScope(Dispatchers.Main)
    val dialogBinding = DialogPickHashtagBinding.inflate(LayoutInflater.from(this))
    val autocompleteTextView = dialogBinding.pickHashtagEditText

    val autoCompleteProvider = object : ComposeAutoCompleteAdapter.AutocompletionProvider {
        override fun search(token: String): List<ComposeAutoCompleteAdapter.AutocompleteResult> {
            return runBlocking {
                api.search(query = token, type = SearchType.Hashtag.apiParameter, limit = 5)
                    .fold({ searchResult ->
                        searchResult.hashtags.map {
                            ComposeAutoCompleteAdapter.AutocompleteResult.HashtagResult(
                                it.name
                            )
                        }
                    }, { e ->
                        Log.e("HashtagPickerDialog", "Autocomplete search for $token failed", e)
                        emptyList()
                    })
            }
        }
    }

    autocompleteTextView.setAdapter(
        ComposeAutoCompleteAdapter(
            autoCompleteProvider,
            animateAvatar = false,
            animateEmojis = false,
            showBotBadge = false,
            withDecoration = false
        )
    )

    autocompleteTextView.setSelection(autocompleteTextView.length())

    val dialog = MaterialAlertDialogBuilder(this)
        .setTitle(title)
        .setView(dialogBinding.root)
        .setPositiveButton(android.R.string.ok) { _, _ ->
            onHashtagSelected(autocompleteTextView.text.toString())
        }
        .setNegativeButton(android.R.string.cancel, null)
        .setOnDismissListener {
            dialogScope.cancel()
        }
        .create()

    autocompleteTextView.doOnTextChanged { s, _, _, _ ->
        dialog.getButton(AlertDialog.BUTTON_POSITIVE).isEnabled = validateHashtag(s)
    }

    autocompleteTextView.setOnEditorActionListener(object : OnEditorActionListener {
        override fun onEditorAction(v: TextView?, actionId: Int, event: KeyEvent?): Boolean {
            if (actionId == EditorInfo.IME_ACTION_DONE && validateHashtag(autocompleteTextView.text)) {
                onHashtagSelected(autocompleteTextView.text.toString())
                dialog.dismiss()
                return true
            }
            return false
        }
    })

    dialog.window?.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_VISIBLE)
    dialog.show()
    dialog.getButton(AlertDialog.BUTTON_POSITIVE).isEnabled = validateHashtag(autocompleteTextView.text)
    autocompleteTextView.requestFocus()
}

private fun validateHashtag(input: CharSequence?): Boolean {
    val trimmedInput = input?.trim() ?: ""
    return trimmedInput.isNotEmpty() && hashtagPattern.matcher(trimmedInput).matches()
}
