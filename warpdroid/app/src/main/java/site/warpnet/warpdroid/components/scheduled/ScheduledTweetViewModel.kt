/* Copyright 2019 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.scheduled

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.TweetScheduledEvent
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.network.WarpnetApi
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
class ScheduledTweetViewModel @Inject constructor(
    private val warpnetApi: WarpnetApi,
    private val eventHub: EventHub,
    accountManager: AccountManager
) : ViewModel() {

    private val refreshTrigger = MutableStateFlow(0)

    private var currentPagingSource: ScheduledTweetPagingSource? = null

    val scheduledStatuses: MutableList<ScheduledTweetViewData> = mutableListOf()
    var nextKey: String? = null

    @OptIn(ExperimentalPagingApi::class, ExperimentalCoroutinesApi::class)
    val data = refreshTrigger.flatMapLatest {
        Pager(
            config = PagingConfig(
                pageSize = 20,
                initialLoadSize = 20
            ),
            pagingSourceFactory = {
                ScheduledTweetPagingSource(this).also { newPagingSource ->
                    this.currentPagingSource = newPagingSource
                }
            },
            remoteMediator = ScheduledTweetRemoteMediator(warpnetApi, accountManager, this)
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
                if (event is TweetScheduledEvent) {
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

    fun deleteScheduledStatus(status: ScheduledTweetViewData) {
        viewModelScope.launch {
            warpnetApi.deleteScheduledStatus(status.id).fold(
                {
                    scheduledStatuses.removeAll { s ->
                        s.id == status.id
                    }
                    invalidate()
                },
                { throwable ->
                    Log.w("ScheduledTweetViewModel", "Error deleting scheduled status", throwable)
                    _error.emit(throwable)
                }
            )
        }
    }
}
