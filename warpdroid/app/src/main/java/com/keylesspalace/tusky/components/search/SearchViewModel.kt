/* Copyright 2021 Tusky Contributors
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

package com.keylesspalace.tusky.components.search

import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.InvalidatingPagingSourceFactory
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import at.connyduck.calladapter.networkresult.NetworkResult
import at.connyduck.calladapter.networkresult.map
import at.connyduck.calladapter.networkresult.onFailure
import com.keylesspalace.tusky.appstore.BlockEvent
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.MuteEvent
import com.keylesspalace.tusky.appstore.PollVoteEvent
import com.keylesspalace.tusky.appstore.StatusChangedEvent
import com.keylesspalace.tusky.appstore.StatusDeletedEvent
import com.keylesspalace.tusky.components.search.paging.SearchPagingSource
import com.keylesspalace.tusky.components.search.paging.SearchStatusPagingSource
import com.keylesspalace.tusky.components.search.paging.SearchStatusRemoteMediator
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Poll
import com.keylesspalace.tusky.entity.Quote
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.network.MastodonApi
import com.keylesspalace.tusky.util.toViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewdata.TranslationViewData
import com.keylesspalace.tusky.viewmodel.StatusActionsViewModel
import dagger.hilt.android.lifecycle.HiltViewModel
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.launch

@HiltViewModel
class SearchViewModel @Inject constructor(
    private val mastodonApi: MastodonApi,
    eventHub: EventHub,
    private val accountManager: AccountManager
) : StatusActionsViewModel(mastodonApi, eventHub) {

    private val _currentQuery: MutableStateFlow<String> = MutableStateFlow("")
    val currentQuery = _currentQuery.asStateFlow()

    var currentSearchFieldContent: String? = null

    val activeAccount: AccountEntity?
        get() = accountManager.activeAccount

    val mediaPreviewEnabled = activeAccount?.mediaPreviewEnabled == true
    val alwaysShowSensitiveMedia = activeAccount?.alwaysShowSensitiveMedia == true
    val alwaysOpenSpoiler = activeAccount?.alwaysOpenSpoiler == true

    val loadedStatuses: MutableList<StatusViewData.Concrete> = mutableListOf()

    val statusesPagingSourceFactory = InvalidatingPagingSourceFactory {
        SearchStatusPagingSource(loadedStatuses, loadedStatuses.size)
    }

    /**
     * Statuses can be changed through local interaction (e.g. favs/boost), so we need a PagingSource and a RemoteMediator.
     * Accounts and Hashtags are only displayed, so a PagingSource is enough.
     */

    @OptIn(ExperimentalCoroutinesApi::class, ExperimentalPagingApi::class)
    val statusesFlow = currentQuery.flatMapLatest { query ->
        Pager(
            config = PagingConfig(
                pageSize = DEFAULT_LOAD_SIZE,
                initialLoadSize = DEFAULT_LOAD_SIZE
            ),
            pagingSourceFactory = statusesPagingSourceFactory,
            remoteMediator = SearchStatusRemoteMediator(
                api = mastodonApi,
                searchRequest = query,
                onPageLoaded = { searchResult ->
                    val statuses = searchResult.statuses.map { status ->
                        status.toViewData(
                            isShowingContent = status.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.PUBLIC),
                            isExpanded = alwaysOpenSpoiler,
                            isCollapsed = true,
                            filterKind = Filter.Kind.PUBLIC,
                            filterActive = true,
                            isQuoteShowingContent =
                            status.quote?.quotedStatus?.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.THREAD)
                                ?: alwaysShowSensitiveMedia,
                            isQuoteExpanded = alwaysOpenSpoiler,
                            isQuoteCollapsed = true,
                            isQuoteShown = status.quote?.state == Quote.State.ACCEPTED
                        )
                    }
                    loadedStatuses.addAll(statuses)
                    statusesPagingSourceFactory.invalidate()
                    statuses.isEmpty()
                },
                currentOffset = { loadedStatuses.size },
            )
        ).flow
    }.cachedIn(viewModelScope)

    @OptIn(ExperimentalCoroutinesApi::class)
    val accountsFlow = currentQuery.flatMapLatest { query ->
        Pager(
            config = PagingConfig(
                pageSize = DEFAULT_LOAD_SIZE,
                initialLoadSize = DEFAULT_LOAD_SIZE
            ),
            pagingSourceFactory = {
                SearchPagingSource(
                    mastodonApi,
                    SearchType.Account,
                    searchRequest = query
                ) {
                    it.accounts
                }
            }
        ).flow
    }
        .cachedIn(viewModelScope)

    @OptIn(ExperimentalCoroutinesApi::class)
    val hashtagsFlow = currentQuery.flatMapLatest { query ->
        Pager(
            config = PagingConfig(
                pageSize = DEFAULT_LOAD_SIZE,
                initialLoadSize = DEFAULT_LOAD_SIZE
            ),
            pagingSourceFactory = {
                SearchPagingSource(
                    mastodonApi,
                    SearchType.Hashtag,
                    searchRequest = query
                ) {
                    it.hashtags
                }
            }
        ).flow
    }
        .cachedIn(viewModelScope)

    init {
        viewModelScope.launch {
            eventHub.events.collect { event ->
                when (event) {
                    is StatusChangedEvent -> handleStatusChangedEvent(event.status)
                    is PollVoteEvent -> handlePollVotedEvent(event.statusId, event.poll)
                    is BlockEvent -> removeAllByAccountId(event.accountId)
                    is MuteEvent -> removeAllByAccountId(event.accountId)
                    is StatusDeletedEvent -> removeStatus(event.statusId)
                }
            }
        }
    }

    fun search(query: String) {
        loadedStatuses.clear()
        _currentQuery.value = query
    }

    fun expandedChange(statusViewData: StatusViewData.Concrete, expanded: Boolean) {
        updateStatusViewData(statusViewData.copy(isExpanded = expanded))
    }

    fun contentHiddenChange(statusViewData: StatusViewData.Concrete, isShowing: Boolean) {
        updateStatusViewData(statusViewData.copy(isShowingContent = isShowing))
    }

    fun collapsedChange(statusViewData: StatusViewData.Concrete, collapsed: Boolean) {
        updateStatusViewData(statusViewData.copy(isCollapsed = collapsed))
    }

    suspend fun translate(statusViewData: StatusViewData.Concrete): NetworkResult<Unit> {
        updateStatusViewData(statusViewData.copy(translation = TranslationViewData.Loading))
        return mastodonApi.translate(statusViewData.actionableId, Locale.getDefault().language)
            .map { translation ->
                updateStatusViewData(
                    statusViewData.copy(
                        translation = TranslationViewData.Loaded(
                            translation
                        )
                    )
                )
            }
            .onFailure {
                updateStatusViewData(statusViewData.copy(translation = null))
            }
    }

    fun untranslate(statusViewData: StatusViewData.Concrete) {
        updateStatusViewData(statusViewData.copy(translation = null))
    }

    fun showPollResults(viewData: StatusViewData.Concrete) {
        updateStatusViewData(
            viewData.copy(
                status = viewData.status.copy(
                    poll = viewData.status.poll?.copy(voted = true)
                )
            )
        )
    }

    fun changeFilter(filtered: Boolean, status: StatusViewData.Concrete) {
        updateStatusViewData(status.copy(filterActive = filtered))
    }

    fun showQuote(viewData: StatusViewData.Concrete) {
        updateStatusViewData(
            viewData.copy(
                quote = viewData.quote?.copy(
                    quoteShown = true
                )
            )
        )
    }

    private fun handleStatusChangedEvent(status: Status) {
        updateStatusViewData(status.id) { viewData ->
            val oldQuoteViewData = viewData.quote?.quotedStatusViewData
            status.toViewData(
                isShowingContent = viewData.isShowingContent,
                isExpanded = viewData.isExpanded,
                isCollapsed = viewData.isCollapsed,
                isDetailed = viewData.isDetailed,
                translation = viewData.translation,
                filterKind = Filter.Kind.THREAD,
                filterActive = viewData.filterActive,
                isQuoteShowingContent = oldQuoteViewData?.isShowingContent
                    ?: status.quote?.quotedStatus?.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.THREAD)
                    ?: alwaysShowSensitiveMedia,
                isQuoteExpanded = oldQuoteViewData?.isExpanded ?: alwaysOpenSpoiler,
                isQuoteCollapsed = oldQuoteViewData?.isCollapsed ?: true,
                isQuoteShown = viewData.quote?.quoteShown ?: false
            )
        }
    }

    private fun handlePollVotedEvent(statusId: String, poll: Poll) {
        updateStatus(statusId) { status ->
            status.copy(poll = poll)
        }
    }

    private fun removeAllByAccountId(accountId: String) {
        if (loadedStatuses.removeAll { it.status.account.id == accountId }) {
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun removeStatus(statusId: String) {
        if (loadedStatuses.removeAll { it.id == statusId }) {
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun updateStatusViewData(newStatusViewData: StatusViewData.Concrete) {
        val idx = loadedStatuses.indexOfFirst { it.id == newStatusViewData.id }
        if (idx >= 0) {
            loadedStatuses[idx] = newStatusViewData
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun updateStatusViewData(
        statusId: String,
        updater: (StatusViewData.Concrete) -> StatusViewData.Concrete
    ) {
        val idx = loadedStatuses.indexOfFirst { it.id == statusId }
        if (idx >= 0) {
            val statusViewData = loadedStatuses[idx]
            loadedStatuses[idx] = updater(statusViewData)
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun updateStatus(statusId: String, updater: (Status) -> Status) {
        updateStatusViewData(statusId) { viewData ->
            viewData.copy(
                status = updater(viewData.status)
            )
        }
    }

    companion object {
        internal const val DEFAULT_LOAD_SIZE = 20
    }
}
