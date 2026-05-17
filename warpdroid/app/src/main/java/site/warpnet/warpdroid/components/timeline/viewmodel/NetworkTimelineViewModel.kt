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

package site.warpnet.warpdroid.components.timeline.viewmodel

import android.content.SharedPreferences
import android.util.Log
import androidx.lifecycle.viewModelScope
import androidx.paging.ExperimentalPagingApi
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import androidx.paging.filter
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.Event
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.MuteEvent
import site.warpnet.warpdroid.appstore.TweetChangedEvent
import site.warpnet.warpdroid.appstore.TweetDeletedEvent
import site.warpnet.warpdroid.appstore.UnfollowEvent
import site.warpnet.warpdroid.components.preference.PreferencesFragment.ReadingOrder.NEWEST_FIRST
import site.warpnet.warpdroid.components.preference.PreferencesFragment.ReadingOrder.OLDEST_FIRST
import site.warpnet.warpdroid.components.timeline.util.ifExpected
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Quote
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.isLessThan
import site.warpnet.warpdroid.util.isLessThanOrEqual
import site.warpnet.warpdroid.util.toViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import dagger.hilt.android.lifecycle.HiltViewModel
import java.io.IOException
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.asExecutor
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import retrofit2.HttpException
import retrofit2.Response

/**
 * TimelineViewModel that caches all statuses in an in-memory list
 */
@HiltViewModel
class NetworkTimelineViewModel @Inject constructor(
    private val api: WarpnetApi,
    eventHub: EventHub,
    accountManager: AccountManager,
    sharedPreferences: SharedPreferences,
) : TimelineViewModel(
    api,
    eventHub,
    accountManager,
    sharedPreferences
) {

    var currentSource: NetworkTimelinePagingSource? = null

    val statusData: MutableList<TweetViewData> = mutableListOf()

    var nextKey: String? = null

    @OptIn(ExperimentalPagingApi::class)
    override val statuses = Pager(
        config = PagingConfig(
            pageSize = LOAD_AT_ONCE
        ),
        pagingSourceFactory = {
            NetworkTimelinePagingSource(
                viewModel = this
            ).also { source ->
                currentSource = source
            }
        },
        remoteMediator = NetworkTimelineRemoteMediator(this)
    ).flow
        .map { pagingData ->
            pagingData.filter(Dispatchers.Default.asExecutor()) { statusViewData ->
                !shouldHideStatus(statusViewData)
            }
        }
        .flowOn(Dispatchers.Default)
        .cachedIn(viewModelScope)

    init {
        viewModelScope.launch {
            eventHub.events
                .collect { event -> handleEvent(event) }
        }
    }

    private fun handleEvent(event: Event) {
        when (event) {
            is TweetChangedEvent -> handleTweetChangedEvent(event.status)
            is UnfollowEvent -> {
                if (kind == Kind.HOME) {
                    val id = event.accountId
                    removeAllByAccountId(id)
                }
            }
            is BlockEvent -> {
                if (kind != Kind.USER && kind != Kind.USER_WITH_REPLIES && kind != Kind.USER_PINNED) {
                    val id = event.accountId
                    removeAllByAccountId(id)
                }
            }
            is MuteEvent -> {
                if (kind != Kind.USER && kind != Kind.USER_WITH_REPLIES && kind != Kind.USER_PINNED) {
                    val id = event.accountId
                    removeAllByAccountId(id)
                }
            }
            is TweetDeletedEvent -> {
                if (kind != Kind.USER && kind != Kind.USER_WITH_REPLIES && kind != Kind.USER_PINNED) {
                    removeStatusWithId(event.statusId)
                }
            }
        }
    }

    override fun changeExpanded(expanded: Boolean, status: TweetViewData.Concrete) {
        status.copy(
            isExpanded = expanded
        ).update()
    }

    override fun changeContentShowing(isShowing: Boolean, status: TweetViewData.Concrete) {
        status.copy(
            isShowingContent = isShowing
        ).update()
    }

    override fun changeContentCollapsed(isCollapsed: Boolean, status: TweetViewData.Concrete) {
        status.copy(
            isCollapsed = isCollapsed
        ).update()
    }

    private fun removeAllByAccountId(accountId: String) {
        statusData.removeAll { vd ->
            val status = vd.asStatusOrNull()?.status ?: return@removeAll false
            status.account.id == accountId || status.actionableStatus.account.id == accountId
        }
        currentSource?.invalidate()
    }

    override fun removeStatusWithId(id: String) {
        statusData.removeAll { vd ->
            val status = vd.asStatusOrNull()?.status ?: return@removeAll false
            status.id == id || status.retweet?.id == id
        }
        currentSource?.invalidate()
    }

    override fun loadMore(placeholderId: String) {
        viewModelScope.launch {
            try {
                val placeholderIndex =
                    statusData.indexOfFirst { it is TweetViewData.LoadMore && it.id == placeholderId }
                statusData[placeholderIndex] =
                    TweetViewData.LoadMore(placeholderId, isLoading = true)
                currentSource?.invalidate()

                val idAbovePlaceholder = statusData.getOrNull(placeholderIndex - 1)?.id
                val idBelowPlaceholder = statusData.getOrNull(placeholderIndex + 1)?.id

                val statusResponse = when (readingOrder) {
                    OLDEST_FIRST -> fetchStatusesForKind(minId = idBelowPlaceholder)
                    NEWEST_FIRST -> fetchStatusesForKind(maxId = idAbovePlaceholder)
                }

                val statuses = statusResponse.body()
                if (!statusResponse.isSuccessful || statuses == null) {
                    loadMoreFailed(placeholderId, HttpException(statusResponse))
                    return@launch
                }

                statusData.removeAt(placeholderIndex)

                val activeAccount = accountManager.activeAccount!!
                val data: MutableList<TweetViewData> = statuses.map { status ->
                    status.toViewData(
                        isShowingContent = status.shouldShowContent(activeAccount.alwaysShowSensitiveMedia, kind.toFilterKind()),
                        isExpanded = activeAccount.alwaysOpenSpoiler,
                        isCollapsed = true,
                        filterKind = kind.toFilterKind(),
                        filterActive = true,
                        isQuoteShowingContent =
                        status.quote?.quotedStatus?.shouldShowContent(activeAccount.alwaysShowSensitiveMedia, kind.toFilterKind())
                            ?: activeAccount.alwaysShowSensitiveMedia,
                        isQuoteExpanded = activeAccount.alwaysOpenSpoiler,
                        isQuoteCollapsed = true,
                        isQuoteShown = status.quote?.state == Quote.State.ACCEPTED
                    )
                }.toMutableList()

                if (statuses.isNotEmpty()) {
                    val firstId = statuses.first().id
                    val lastId = statuses.last().id
                    val overlappedFrom = statusData.indexOfFirst {
                        it.asStatusOrNull()?.id?.isLessThanOrEqual(firstId) ?: false
                    }
                    val overlappedTo = statusData.indexOfFirst {
                        it.asStatusOrNull()?.id?.isLessThan(lastId) ?: false
                    } - 1

                    if (overlappedFrom < overlappedTo) {
                        data.mapIndexed { i, status ->
                            i to statusData.firstOrNull {
                                it.asStatusOrNull()?.id == status.id
                            }?.asStatusOrNull()
                        }
                            .filter { (_, oldStatus) -> oldStatus != null }
                            .forEach { (i, oldStatus) ->
                                data[i] = data[i].asStatusOrNull()!!
                                    .copy(
                                        isShowingContent = oldStatus!!.isShowingContent,
                                        isExpanded = oldStatus.isExpanded,
                                        isCollapsed = oldStatus.isCollapsed
                                    )
                            }
                        statusData.removeAll { status ->
                            lastId.isLessThanOrEqual(status.id) && status.id.isLessThanOrEqual(firstId)
                        }
                        statusData.addAll(overlappedFrom, data)
                    } else {
                        when (readingOrder) {
                            OLDEST_FIRST -> {
                                data[0] = TweetViewData.LoadMore(statuses.first().id, isLoading = false)
                            }
                            NEWEST_FIRST -> {
                                data[data.size - 1] = TweetViewData.LoadMore(statuses.last().id, isLoading = false)
                            }
                        }
                        statusData.addAll(placeholderIndex, data)
                    }
                }

                currentSource?.invalidate()
            } catch (e: Exception) {
                ifExpected(e) {
                    loadMoreFailed(placeholderId, e)
                }
            }
        }
    }

    private fun loadMoreFailed(placeholderId: String, e: Exception) {
        Log.w("NetworkTimelineVM", "failed loading statuses", e)

        val index =
            statusData.indexOfFirst { it is TweetViewData.LoadMore && it.id == placeholderId }
        statusData[index] = TweetViewData.LoadMore(placeholderId, isLoading = false)

        currentSource?.invalidate()
    }

    private fun handleTweetChangedEvent(status: Tweet) {
        updateStatusByActionableId(status.id) { status }
    }

    override fun fullReload() {
        nextKey = statusData.firstOrNull { it is TweetViewData.Concrete }?.asStatusOrNull()?.id
        statusData.clear()
        currentSource?.invalidate()
    }

    override fun changeFilter(filtered: Boolean, status: TweetViewData.Concrete) {
        status.copy(filterActive = filtered).update()
    }

    override fun showQuote(status: TweetViewData.Concrete) {
        status.copy(
            quote = status.quote?.copy(
                quoteShown = true
            )
        ).update()
    }

    override fun saveHomeTimelinePosition(firstVisibleIndex: Int, firstVisibleOffset: Int) {
        /** Does nothing for non-cached timelines */
    }

    override suspend fun invalidate() {
        currentSource?.invalidate()
    }

    @Throws(IOException::class, HttpException::class)
    suspend fun fetchStatusesForKind(
        maxId: String? = null,
        minId: String? = null,
        sinceId: String? = null,
        limit: Int = LOAD_AT_ONCE
    ): Response<List<Tweet>> {
        return when (kind) {
            Kind.HOME -> api.homeTimeline(
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit
            )
            Kind.USER -> api.accountStatuses(
                accountId = id!!,
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit,
                excludeReplies = true,
                onlyMedia = null,
                pinned = null
            )

            Kind.USER_PINNED -> api.accountStatuses(
                accountId = id!!,
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit,
                excludeReplies = null,
                onlyMedia = null,
                pinned = true
            )

            Kind.USER_WITH_REPLIES -> api.accountStatuses(
                accountId = id!!,
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit,
                excludeReplies = null,
                onlyMedia = null,
                pinned = null
            )

            Kind.LIKES -> api.likes(
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit
            )
            Kind.BOOKMARKS -> api.bookmarks(
                maxId = maxId,
                minId = minId,
                sinceId = sinceId,
                limit = limit
            )
            Kind.QUOTES -> api.quotingStatuses(
                statusId = id!!,
                limit = limit,
                offset = maxId
            )
        }
    }

    private fun TweetViewData.Concrete.update() {
        val position =
            statusData.indexOfFirst { viewData -> viewData.asStatusOrNull()?.id == this.id }
        if (position >= 0) {
            statusData[position] = this
        } else {
            val position =
                statusData.indexOfFirst { viewData ->
                    viewData.asStatusOrNull()?.quote?.quotedTweetViewData?.id == this.id
                }
            if (position != -1) {
                statusData[position].asStatusOrNull()?.let { viewData ->
                    statusData[position] = viewData.copy(
                        quote = viewData.quote?.copy(
                            quotedTweetViewData = this
                        )
                    )
                }
            }
        }
        currentSource?.invalidate()
    }

    private inline fun updateStatusByActionableId(id: String, updater: (Tweet) -> Tweet) {
        // posts can be multiple times in the timeline, e.g. once the original and once as retweet
        statusData.forEachIndexed { index, viewData ->
            val status = viewData.asStatusOrNull()
            if (status?.actionableId == id) {
                updateViewDataAt(index) { vd ->
                    if (vd.status.retweet != null) {
                        vd.copy(status = vd.status.copy(retweet = updater(vd.status.retweet)))
                    } else {
                        vd.copy(status = updater(vd.status))
                    }
                }
            } else if (status?.quote?.quotedTweetViewData?.id == id) {
                updateViewDataAt(index) { vd ->
                    if (vd.status.retweet != null) {
                        vd.copy(
                            status = vd.status.copy(
                                retweet = vd.status.retweet.copy(
                                    quote = vd.status.retweet.quote?.copy(
                                        quotedStatus = vd.status.retweet.quote.quotedStatus?.let { updater(it) }
                                    )
                                )
                            ),
                            quote = vd.quote?.copy(
                                quotedTweetViewData = vd.quote.quotedTweetViewData?.copy(
                                    status = updater(vd.quote.quotedTweetViewData.status)
                                )
                            )
                        )
                    } else {
                        vd.copy(
                            status = vd.status.copy(
                                quote = vd.status.quote?.copy(
                                    quotedStatus = vd.status.quote.quotedStatus?.let { updater(it) }
                                )
                            ),
                            quote = vd.quote?.copy(
                                quotedTweetViewData = vd.quote.quotedTweetViewData?.copy(
                                    status = updater(vd.quote.quotedTweetViewData.status)
                                )
                            )
                        )
                    }
                }
            }
        }
    }

    private inline fun updateViewDataAt(
        position: Int,
        updater: (TweetViewData.Concrete) -> TweetViewData.Concrete
    ) {
        val status = statusData.getOrNull(position)?.asStatusOrNull() ?: return
        statusData[position] = updater(status)
        currentSource?.invalidate()
    }
}
