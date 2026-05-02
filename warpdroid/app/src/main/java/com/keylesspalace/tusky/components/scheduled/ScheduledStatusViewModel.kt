/* Copyright 2019 Tusky Contributors
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

package com.keylesspalace.tusky.components.scheduled

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import at.connyduck.calladapter.networkresult.fold
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.StatusScheduledEvent
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.network.MastodonApi
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

@HiltViewModel
class ScheduledStatusViewModel @Inject constructor(
    private val mastodonApi: MastodonApi,
    private val eventHub: EventHub,
    accountManager: AccountManager
) : ViewModel() {

    private val refreshTrigger = MutableStateFlow(0)

    private var currentPagingSource: ScheduledStatusPagingSource? = null

    val scheduledStatuses: MutableList<ScheduledStatusViewData> = mutableListOf()
    var nextKey: String? = null

    @OptIn(ExperimentalPagingApi::class, ExperimentalCoroutinesApi::class)
    val data = refreshTrigger.flatMapLatest {
        Pager(
            config = PagingConfig(
                pageSize = 20,
                initialLoadSize = 20
            ),
            pagingSourceFactory = {
                ScheduledStatusPagingSource(this).also { newPagingSource ->
                    this.currentPagingSource = newPagingSource
                }
            },
            remoteMediator = ScheduledStatusRemoteMediator(mastodonApi, accountManager, this)
        ).flow
            .cachedIn(viewModelScope)
    }

    private val _error = MutableSharedFlow<Throwable>(
        replay = 0,
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val error: SharedFlow<Throwable> = _error.asSharedFlow()

    init {
        viewModelScope.launch {
            eventHub.events.collect { event ->
                if (event is StatusScheduledEvent) {
                    refreshTrigger.update { it + 1 }
                }
            }
        }
    }

    fun invalidate() {
        currentPagingSource?.invalidate()
    }

    fun expandSpoiler(expanded: Boolean, id: String) {
        scheduledStatuses.replaceAll { status ->
            if (id == status.id) {
                status.copy(spoilerExpanded = expanded)
            } else {
                status
            }
        }
        invalidate()
    }

    fun showOverflow(overflowVisible: Boolean, id: String) {
        scheduledStatuses.replaceAll { status ->
            if (id == status.id) {
                status.copy(overflowVisible = overflowVisible)
            } else {
                status
            }
        }
        invalidate()
    }

    fun showMedia(show: Boolean, id: String) {
        scheduledStatuses.replaceAll { status ->
            if (id == status.id) {
                status.copy(mediaVisible = show)
            } else {
                status
            }
        }
        invalidate()
    }

    fun deleteScheduledStatus(status: ScheduledStatusViewData) {
        viewModelScope.launch {
            mastodonApi.deleteScheduledStatus(status.id).fold(
                {
                    scheduledStatuses.removeAll { s ->
                        s.id == status.id
                    }
                    invalidate()
                },
                { throwable ->
                    Log.w("ScheduledStatusViewModel", "Error deleting scheduled status", throwable)
                    _error.emit(throwable)
                }
            )
        }
    }
}
