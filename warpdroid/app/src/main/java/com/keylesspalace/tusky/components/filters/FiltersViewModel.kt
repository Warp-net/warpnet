package com.keylesspalace.tusky.components.filters

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.FilterUpdatedEvent
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.network.MastodonApi
import dagger.hilt.android.lifecycle.HiltViewModel
import java.text.Collator
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

@HiltViewModel
class FiltersViewModel @Inject constructor(
    private val api: MastodonApi,
    private val eventHub: EventHub
) : ViewModel() {

    data class State(
        val filters: List<Filter> = emptyList(),
        val loading: Boolean = true,
        val error: Throwable? = null,
        val deletionError: Filter? = null
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null

    init {
        reload()
    }

    fun reload() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }

            val collator = Collator.getInstance()
            api.getFilters().fold(
                { filters ->
                    _state.value = State(
                        /* Sorting the filters alphabetically while ignoring special characters.
                           This makes #hashtag and @username filters be in between other filters. */
                        filters = filters.sortedWith { a, b ->
                            collator.compare(a.title.removeSpecialChars(), b.title.removeSpecialChars())
                        },
                        loading = false,
                        error = null
                    )
                },
                { throwable ->
                    Log.w(TAG, "failed loading filters", throwable)
                    _state.update { it.copy(filters = emptyList(), loading = false, error = throwable) }
                }
            )
        }
    }

    fun deleteFilter(filter: Filter) {
        viewModelScope.launch {
            // First wait for a pending loading operation to complete
            _state.first { !it.loading }

            api.deleteFilter(filter.id).fold(
                {
                    _state.update { currentState ->
                        State(
                            filters = currentState.filters.filter { it.id != filter.id },
                            loading = false
                        )
                    }
                    eventHub.dispatch(FilterUpdatedEvent(filter.context))
                },
                { throwable ->
                    Log.w(TAG, "failed to delete filter", throwable)
                    _state.update { it.copy(deletionError = filter) }
                }
            )
        }
    }

    fun clearDeletionError() {
        _state.update { it.copy(deletionError = null) }
    }

    private fun String.removeSpecialChars() =
        filter { it.isLetterOrDigit() }

    companion object {
        private const val TAG = "FiltersViewModel"
    }
}
