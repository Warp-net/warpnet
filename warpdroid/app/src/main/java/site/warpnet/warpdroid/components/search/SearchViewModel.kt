/* Copyright 2021 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.search

import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.InvalidatingPagingSourceFactory
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.MuteEvent
import site.warpnet.warpdroid.appstore.TweetChangedEvent
import site.warpnet.warpdroid.appstore.TweetDeletedEvent
import site.warpnet.warpdroid.components.search.paging.SearchPagingSource
import site.warpnet.warpdroid.components.search.paging.SearchTweetPagingSource
import site.warpnet.warpdroid.components.search.paging.SearchTweetRemoteMediator
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Quote
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.toViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import site.warpnet.warpdroid.viewmodel.TweetActionsViewModel
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.launch

@HiltViewModel
class SearchViewModel @Inject constructor(
    private val warpnetApi: WarpnetApi,
    eventHub: EventHub,
    private val accountManager: AccountManager
) : TweetActionsViewModel(warpnetApi, eventHub) {

    private val _currentQuery: MutableStateFlow<String> = MutableStateFlow("")
    val currentQuery = _currentQuery.asStateFlow()

    var currentSearchFieldContent: String? = null

    val activeAccount: AccountEntity?
        get() = accountManager.activeAccount

    val mediaPreviewEnabled = activeAccount?.mediaPreviewEnabled == true
    val alwaysShowSensitiveMedia = activeAccount?.alwaysShowSensitiveMedia == true
    val alwaysOpenSpoiler = activeAccount?.alwaysOpenSpoiler == true

    val loadedStatuses: MutableList<TweetViewData.Concrete> = mutableListOf()

    val statusesPagingSourceFactory = InvalidatingPagingSourceFactory {
        SearchTweetPagingSource(loadedStatuses, loadedStatuses.size)
    }

    /**
     * Statuses can be changed through local interaction (e.g. favs/retweet), so we need a PagingSource and a RemoteMediator.
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
            remoteMediator = SearchTweetRemoteMediator(
                api = warpnetApi,
                searchRequest = query,
                onPageLoaded = { searchResult ->
                    val statuses = searchResult.statuses.map { status ->
                        status.toViewData(
                            isShowingContent = status.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.HOME),
                            isExpanded = alwaysOpenSpoiler,
                            isCollapsed = true,
                            filterKind = Filter.Kind.HOME,
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
                    warpnetApi,
                    SearchType.User,
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
                    warpnetApi,
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
                    is TweetChangedEvent -> handleTweetChangedEvent(event.status)
                    is BlockEvent -> removeAllByAccountId(event.accountId)
                    is MuteEvent -> removeAllByAccountId(event.accountId)
                    is TweetDeletedEvent -> removeStatus(event.statusId)
                }
            }
        }
    }

    fun search(query: String) {
        loadedStatuses.clear()
        _currentQuery.value = query
    }

    fun expandedChange(statusViewData: TweetViewData.Concrete, expanded: Boolean) {
        updateTweetViewData(statusViewData.copy(isExpanded = expanded))
    }

    fun contentHiddenChange(statusViewData: TweetViewData.Concrete, isShowing: Boolean) {
        updateTweetViewData(statusViewData.copy(isShowingContent = isShowing))
    }

    fun collapsedChange(statusViewData: TweetViewData.Concrete, collapsed: Boolean) {
        updateTweetViewData(statusViewData.copy(isCollapsed = collapsed))
    }

    fun changeFilter(filtered: Boolean, status: TweetViewData.Concrete) {
        updateTweetViewData(status.copy(filterActive = filtered))
    }

    fun showQuote(viewData: TweetViewData.Concrete) {
        updateTweetViewData(
            viewData.copy(
                quote = viewData.quote?.copy(
                    quoteShown = true
                )
            )
        )
    }

    private fun handleTweetChangedEvent(status: Tweet) {
        updateTweetViewData(status.id) { viewData ->
            val oldQuoteViewData = viewData.quote?.quotedTweetViewData
            status.toViewData(
                isShowingContent = viewData.isShowingContent,
                isExpanded = viewData.isExpanded,
                isCollapsed = viewData.isCollapsed,
                isDetailed = viewData.isDetailed,
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

    private fun updateTweetViewData(newTweetViewData: TweetViewData.Concrete) {
        val idx = loadedStatuses.indexOfFirst { it.id == newTweetViewData.id }
        if (idx >= 0) {
            loadedStatuses[idx] = newTweetViewData
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun updateTweetViewData(
        statusId: String,
        updater: (TweetViewData.Concrete) -> TweetViewData.Concrete
    ) {
        val idx = loadedStatuses.indexOfFirst { it.id == statusId }
        if (idx >= 0) {
            val statusViewData = loadedStatuses[idx]
            loadedStatuses[idx] = updater(statusViewData)
            statusesPagingSourceFactory.invalidate()
        }
    }

    private fun updateStatus(statusId: String, updater: (Tweet) -> Tweet) {
        updateTweetViewData(statusId) { viewData ->
            viewData.copy(
                status = updater(viewData.status)
            )
        }
    }

    companion object {
        internal const val DEFAULT_LOAD_SIZE = 20
    }
}
