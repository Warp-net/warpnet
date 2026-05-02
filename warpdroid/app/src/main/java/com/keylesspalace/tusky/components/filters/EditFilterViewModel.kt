/* Copyright 2024 Tusky contributors
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

package com.keylesspalace.tusky.components.filters

import android.content.Context
import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.FilterUpdatedEvent
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.FilterKeyword
import com.keylesspalace.tusky.network.MastodonApi
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

@HiltViewModel(assistedFactory = EditFilterViewModel.Factory::class)
class EditFilterViewModel @AssistedInject constructor(
    private val api: MastodonApi,
    private val eventHub: EventHub,
    @ApplicationContext private val context: Context,
    @Assisted("originalFilter") val originalFilter: Filter?
) : ViewModel() {

    private val _title: MutableStateFlow<String> = MutableStateFlow(originalFilter?.title.orEmpty())
    val title: StateFlow<String> = _title.asStateFlow()

    private val _keywords: MutableStateFlow<List<FilterKeyword>> = MutableStateFlow(originalFilter?.keywords.orEmpty())
    val keywords: StateFlow<List<FilterKeyword>> = _keywords.asStateFlow()

    private val _action: MutableStateFlow<Filter.Action> = MutableStateFlow(originalFilter?.action ?: Filter.Action.WARN)
    val action: StateFlow<Filter.Action> = _action.asStateFlow()

    private val _duration: MutableStateFlow<Int> = MutableStateFlow(
        if (originalFilter?.expiresAt == null) {
            0
        } else {
            -1
        }
    )
    val duration: StateFlow<Int> = _duration.asStateFlow()

    private val _contexts: MutableStateFlow<List<Filter.Kind>> = MutableStateFlow(originalFilter?.context.orEmpty())
    val contexts: StateFlow<List<Filter.Kind>> = _contexts.asStateFlow()

    private val _state: MutableStateFlow<State> = MutableStateFlow(State.Ready)
    val state: StateFlow<State> = _state.asStateFlow()

    fun addKeyword(keyword: FilterKeyword) {
        _keywords.value += keyword
    }

    fun deleteKeyword(keyword: FilterKeyword) {
        _keywords.value = _keywords.value.filterNot { it == keyword }
    }

    fun modifyKeyword(original: FilterKeyword, updated: FilterKeyword) {
        val index = _keywords.value.indexOf(original)
        if (index >= 0) {
            _keywords.value = _keywords.value.toMutableList().apply {
                set(index, updated)
            }
        }
    }

    fun setTitle(title: String) {
        this._title.value = title
    }

    fun setDuration(index: Int) {
        _duration.value = index
    }

    fun setAction(action: Filter.Action) {
        this._action.value = action
    }

    fun addContext(context: Filter.Kind) {
        if (!_contexts.value.contains(context)) {
            _contexts.value += context
        }
    }

    fun removeContext(context: Filter.Kind) {
        _contexts.value = _contexts.value.filter { it != context }
    }

    fun validate(): Boolean {
        return _title.value.isNotBlank() &&
            _keywords.value.isNotEmpty() &&
            _contexts.value.isNotEmpty()
    }

    fun saveChanges() {
        _state.value = State.Loading

        val contexts = _contexts.value
        val title = _title.value
        val durationIndex = _duration.value
        val action = _action.value

        viewModelScope.launch {
            val success = originalFilter?.let { filter ->
                updateFilter(filter, title, contexts, action, durationIndex)
            } ?: createFilter(title, contexts, action, durationIndex)

            if (success) {
                val affectedContexts = contexts.union(originalFilter?.context.orEmpty())
                    .distinct()
                eventHub.dispatch(FilterUpdatedEvent(affectedContexts))
                _state.value = State.Finished
            } else {
                _state.value = State.SavingFailed
            }
        }
    }

    fun deleteFilter() {
        originalFilter?.let { filter ->

            _state.value = State.Loading

            viewModelScope.launch {
                api.deleteFilter(filter.id).fold(
                    {
                        _state.value = State.Finished
                    },
                    { throwable ->
                        Log.w(TAG, "failed deleting filter", throwable)
                        _state.value = State.DeletingFailed
                    }
                )
            }
        }
    }

    fun clearError() {
        _state.value = State.Ready
    }

    private suspend fun createFilter(
        title: String,
        contexts: List<Filter.Kind>,
        action: Filter.Action,
        durationIndex: Int
    ): Boolean {
        val expiration = getExpirationForDurationIndex(durationIndex)
        api.createFilter(
            title = title,
            context = contexts,
            filterAction = action,
            expiresIn = expiration
        ).fold(
            { newFilter ->
                // This is _terrible_, but the all-in-one update filter api Just Doesn't Work
                return _keywords.value.map { keyword ->
                    api.addFilterKeyword(
                        filterId = newFilter.id,
                        keyword = keyword.keyword,
                        wholeWord = keyword.wholeWord
                    )
                }.none { it.isFailure }
            },
            { throwable ->
                Log.w(TAG, "failed to create filter", throwable)
                return false
            }
        )
    }

    private suspend fun updateFilter(
        originalFilter: Filter,
        title: String,
        contexts: List<Filter.Kind>,
        action: Filter.Action,
        durationIndex: Int
    ): Boolean {
        val expiration = getExpirationForDurationIndex(durationIndex)
        api.updateFilter(
            id = originalFilter.id,
            title = title,
            context = contexts,
            filterAction = action,
            expires = expiration
        ).fold(
            {
                // This is _terrible_, but the all-in-one update filter api Just Doesn't Work
                val results = _keywords.value.map { keyword ->
                    if (keyword.id.isEmpty()) {
                        api.addFilterKeyword(filterId = originalFilter.id, keyword = keyword.keyword, wholeWord = keyword.wholeWord)
                    } else {
                        api.updateFilterKeyword(keywordId = keyword.id, keyword = keyword.keyword, wholeWord = keyword.wholeWord)
                    }
                } + originalFilter.keywords.filter { keyword ->
                    // Deleted keywords
                    _keywords.value.none { it.id == keyword.id }
                }.map { api.deleteFilterKeyword(it.id) }

                return results.none { it.isFailure }
            },
            { throwable ->
                Log.w(TAG, "failed to update filter", throwable)
                return false
            }
        )
    }

    // Mastodon *stores* the absolute date in the filter,
    // but create/edit take a number of seconds (relative to the time the operation is posted)
    private fun getExpirationForDurationIndex(index: Int): FilterExpiration? {
        return when (index) {
            -1 -> FilterExpiration.unchanged
            0 -> FilterExpiration.never
            else -> FilterExpiration.seconds(
                context.resources.getIntArray(R.array.filter_duration_values)[index]
            )
        }
    }

    sealed class State {
        data object Ready : State()
        data object SavingFailed : State()
        data object DeletingFailed : State()
        data object Loading : State()
        data object Finished : State()
    }

    @AssistedFactory
    interface Factory {
        fun create(
            @Assisted("originalFilter") originalFilter: Filter?
        ): EditFilterViewModel
    }

    companion object {
        private const val TAG = "EditFilterViewModel"
    }
}
