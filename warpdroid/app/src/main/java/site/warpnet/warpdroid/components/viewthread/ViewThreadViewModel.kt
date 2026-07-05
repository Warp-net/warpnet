/* Copyright 2022 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.viewthread

import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import at.connyduck.calladapter.networkresult.getOrElse
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.TweetChangedEvent
import site.warpnet.warpdroid.appstore.TweetComposedEvent
import site.warpnet.warpdroid.appstore.TweetDeletedEvent
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.ui.SnackbarError
import site.warpnet.warpdroid.util.toViewData
import site.warpnet.warpdroid.viewdata.TweetViewData
import site.warpnet.warpdroid.viewmodel.TweetActionsViewModel
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import timber.log.Timber

@HiltViewModel(assistedFactory = ViewThreadViewModel.Factory::class)
class ViewThreadViewModel @AssistedInject constructor(
    private val api: WarpnetApi,
    private val eventHub: EventHub,
    accountManager: AccountManager,
    @Assisted("threadId") val threadId: String,
    @Assisted("authorId") val authorId: String,
) : TweetActionsViewModel(api, eventHub) {

    private val activeAccount = accountManager.activeAccount!!

    private val _uiState: MutableStateFlow<ThreadUiState> = MutableStateFlow(ThreadUiState.Loading)
    val uiState: StateFlow<ThreadUiState> = _uiState.asStateFlow()

    private val _finish: MutableSharedFlow<Unit> = MutableSharedFlow()
    val finish: SharedFlow<Unit> = _finish.asSharedFlow()

    private val alwaysShowSensitiveMedia: Boolean = activeAccount.alwaysShowSensitiveMedia
    private val alwaysOpenSpoiler: Boolean = activeAccount.alwaysOpenSpoiler

    init {
        viewModelScope.launch {
            eventHub.events
                .collect { event ->
                    when (event) {
                        is TweetChangedEvent -> handleTweetChangedEvent(event.status)
                        is BlockEvent -> removeAllByAccountId(event.accountId)
                        is TweetComposedEvent -> handleTweetComposedEvent(event)
                        is TweetDeletedEvent -> removeStatus(event.statusId)
                    }
                }
        }
        loadThread()
    }

    fun retry() {
        _uiState.value = ThreadUiState.Loading
        loadThread()
    }

    fun refresh() {
        var refreshable = false
        _uiState.update { uiState ->
            if (uiState is ThreadUiState.Success) {
                refreshable = true
                uiState.copy(
                    isRefreshing = true
                )
            } else {
                uiState
            }
        }
        if (refreshable) {
            loadThread()
        }
    }

    private fun loadThread() {
        viewModelScope.launch {
            val contextCall = async { api.statusContext(threadId, authorId) }

            // Warpdroid: no local timeline cache — always load detailed status from network.
            Timber.tag(TAG).d("Loaded status from network")
            val result = api.status(threadId, authorId).getOrElse { exception ->
                _uiState.value = ThreadUiState.Error(exception)
                return@launch
            }
            val detailedStatus = result.toViewData(isDetailed = true)

            _uiState.update { uiState ->
                if (uiState is ThreadUiState.Success) {
                    uiState.copy(
                        statusViewData = uiState.ancestors + detailedStatus + uiState.descendants,
                        isRefreshing = false,
                        isloadingThread = true
                    )
                } else {
                    ThreadUiState.Success(
                        statusViewData = listOf(detailedStatus),
                        revealButton = detailedStatus.getRevealButtonState(),
                        isRefreshing = false,
                        isloadingThread = true
                    )
                }
            }

            // let other views know about possible changes to the loaded status
            eventHub.dispatch(TweetChangedEvent(detailedStatus.status))

            val contextResult = contextCall.await()

            contextResult.fold({ statusContext ->
                val ancestors =
                    statusContext.ancestors.map { status -> status.toViewData() }.filter()
                val descendants =
                    statusContext.descendants.map { status -> status.toViewData() }.filter()
                val statuses = ancestors + detailedStatus + descendants

                _uiState.value = ThreadUiState.Success(
                    statusViewData = statuses,
                    revealButton = statuses.getRevealButtonState(),
                    isRefreshing = false,
                    isloadingThread = false
                )
            }, { throwable ->
                Timber.tag(TAG).w(throwable, "Failed to load status context")
                errors.emit(
                    SnackbarError.ResourceMessage(
                        message = R.string.error_generic,
                        retryAction = ::retry
                    )
                )
                _uiState.update { uiState ->
                    if (uiState is ThreadUiState.Success) {
                        uiState.copy(
                            statusViewData = uiState.ancestors + detailedStatus + uiState.descendants,
                            isRefreshing = false,
                            isloadingThread = false
                        )
                    } else {
                        ThreadUiState.Success(
                            statusViewData = listOf(detailedStatus),
                            revealButton = detailedStatus.getRevealButtonState(),
                            isRefreshing = false,
                            isloadingThread = false
                        )
                    }
                }
            })
        }
    }

    fun removeStatus(statusIdToRemove: String) {
        updateSuccess { uiState ->
            uiState.copy(
                statusViewData = uiState.statusViewData.filterNot { status -> status.id == statusIdToRemove }
            )
        }
    }

    fun changeExpanded(expanded: Boolean, status: TweetViewData.Concrete) {
        updateSuccess { uiState ->
            val statuses = uiState.statusViewData.map { viewData ->
                if (viewData.id == status.id) {
                    viewData.copy(isExpanded = expanded)
                } else {
                    if (viewData.quote?.quotedTweetViewData?.id == status.id) {
                        viewData.copy(
                            quote = viewData.quote.copy(
                                quotedTweetViewData = viewData.quote.quotedTweetViewData.copy(isExpanded = expanded)
                            )
                        )
                    } else {
                        viewData
                    }
                }
            }
            uiState.copy(
                statusViewData = statuses,
                revealButton = statuses.getRevealButtonState()
            )
        }
    }

    fun changeContentShowing(isShowing: Boolean, status: TweetViewData.Concrete) {
        updateTweetViewData(status.id) { viewData ->
            viewData.copy(isShowingContent = isShowing)
        }
    }

    fun changeContentCollapsed(isCollapsed: Boolean, status: TweetViewData.Concrete) {
        updateTweetViewData(status.id) { viewData ->
            viewData.copy(isCollapsed = isCollapsed)
        }
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
        updateSuccess { uiState ->
            uiState.copy(
                statusViewData = uiState.statusViewData.filter { viewData ->
                    viewData.status.account.id != accountId
                }
            )
        }
    }

    private fun handleTweetComposedEvent(event: TweetComposedEvent) {
        val eventStatus = event.status
        updateSuccess { uiState ->
            val statuses = uiState.statusViewData
            val detailedIndex = statuses.indexOfFirst { status -> status.isDetailed }
            val repliedIndex =
                statuses.indexOfFirst { status -> eventStatus.inReplyToId == status.id }
            if (detailedIndex != -1 && repliedIndex >= detailedIndex) {
                // there is a new reply to the detailed status or below -> display it
                val newStatuses = statuses.subList(0, repliedIndex + 1) +
                    eventStatus.toViewData() +
                    statuses.subList(repliedIndex + 1, statuses.size)
                uiState.copy(statusViewData = newStatuses)
            } else {
                uiState
            }
        }
    }

    fun toggleRevealButton() {
        updateSuccess { uiState ->
            when (uiState.revealButton) {
                RevealButtonState.HIDE -> uiState.copy(
                    statusViewData = uiState.statusViewData.map { viewData ->
                        viewData.copy(isExpanded = false)
                    },
                    revealButton = RevealButtonState.REVEAL
                )

                RevealButtonState.REVEAL -> uiState.copy(
                    statusViewData = uiState.statusViewData.map { viewData ->
                        viewData.copy(isExpanded = true)
                    },
                    revealButton = RevealButtonState.HIDE
                )

                else -> uiState
            }
        }
    }

    private fun TweetViewData.Concrete.getRevealButtonState(): RevealButtonState {
        val hasWarnings = status.spoilerText.isNotEmpty()

        return if (hasWarnings) {
            if (isExpanded) {
                RevealButtonState.HIDE
            } else {
                RevealButtonState.REVEAL
            }
        } else {
            RevealButtonState.NO_BUTTON
        }
    }

    /**
     * Get the reveal button state based on the state of all the statuses in the list.
     *
     * - If any status sets it to REVEAL, use REVEAL
     * - If no status sets it to REVEAL, but at least one uses HIDE, use HIDE
     * - Otherwise use NO_BUTTON
     */
    private fun List<TweetViewData.Concrete>.getRevealButtonState(): RevealButtonState {
        var seenHide = false

        forEach {
            when (val state = it.getRevealButtonState()) {
                RevealButtonState.NO_BUTTON -> return@forEach
                RevealButtonState.REVEAL -> return state
                RevealButtonState.HIDE -> seenHide = true
            }
        }

        if (seenHide) {
            return RevealButtonState.HIDE
        }

        return RevealButtonState.NO_BUTTON
    }

    private fun List<TweetViewData.Concrete>.filter(): List<TweetViewData.Concrete> {
        return filter { status ->
            if (status.isDetailed || status.status.account.id == activeAccount.accountId) {
                true
            } else {
                !status.isFilterHide
            }
        }
    }

    private fun Tweet.toViewData(isDetailed: Boolean = false): TweetViewData.Concrete {
        val oldStatus = (_uiState.value as? ThreadUiState.Success)?.statusViewData?.find {
            it.id == this.id
        }
        val oldQuoteViewData = oldStatus?.quote?.quotedTweetViewData
        return toViewData(
            isShowingContent = oldStatus?.isShowingContent ?: actionableStatus.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.THREAD),
            isExpanded = oldStatus?.isExpanded ?: alwaysOpenSpoiler,
            isCollapsed = oldStatus?.isCollapsed ?: !isDetailed,
            isDetailed = oldStatus?.isDetailed ?: isDetailed,
            filterKind = Filter.Kind.THREAD,
            filterActive = oldStatus?.filterActive ?: true,
            isQuoteShowingContent = oldQuoteViewData?.isShowingContent
                ?: quote?.quotedStatus?.shouldShowContent(alwaysShowSensitiveMedia, Filter.Kind.THREAD)
                ?: alwaysShowSensitiveMedia,
            isQuoteExpanded = oldQuoteViewData?.isExpanded ?: alwaysOpenSpoiler,
            isQuoteCollapsed = oldQuoteViewData?.isCollapsed ?: true,
            isQuoteShown = oldStatus?.quote?.quoteShown ?: false
        )
    }

    private inline fun updateSuccess(updater: (ThreadUiState.Success) -> ThreadUiState.Success) {
        _uiState.update { uiState ->
            if (uiState is ThreadUiState.Success) {
                updater(uiState)
            } else {
                uiState
            }
        }
    }

    private fun updateTweetViewData(
        statusId: String,
        updater: (TweetViewData.Concrete) -> TweetViewData.Concrete
    ) {
        updateSuccess { uiState ->
            uiState.copy(
                statusViewData = uiState.statusViewData.map { viewData ->
                    if (viewData.id == statusId) {
                        updater(viewData)
                    } else {
                        if (viewData.quote?.quotedTweetViewData?.id == statusId) {
                            viewData.copy(
                                quote = viewData.quote.copy(
                                    quotedTweetViewData = updater(viewData.quote.quotedTweetViewData)
                                )
                            )
                        } else {
                            viewData
                        }
                    }
                }
            )
        }
    }

    fun changeFilter(filtered: Boolean, viewData: TweetViewData.Concrete) {
        updateTweetViewData(viewData.id) { viewData ->
            viewData.copy(
                filterActive = filtered
            )
        }
    }

    fun showQuote(viewData: TweetViewData.Concrete) {
        updateTweetViewData(viewData.id) {
            it.copy(
                quote = it.quote?.copy(quoteShown = true)
            )
        }
    }

    @AssistedFactory
    interface Factory {
        fun create(
            @Assisted("threadId") threadId: String,
            @Assisted("authorId") authorId: String,
        ): ViewThreadViewModel
    }

    companion object {
        private const val TAG = "ViewThreadViewModel"
    }
}

sealed interface ThreadUiState {

    val revealButton: RevealButtonState

    /** The initial load of the detailed status for this thread */
    data object Loading : ThreadUiState {
        override val revealButton: RevealButtonState
            get() = RevealButtonState.NO_BUTTON
    }

    /** No statuses could be loaded from network or cache */
    class Error(val throwable: Throwable) : ThreadUiState {
        override val revealButton: RevealButtonState
            get() = RevealButtonState.NO_BUTTON
    }

    /** Successfully loaded the full thread */
    data class Success(
        val statusViewData: List<TweetViewData.Concrete>,
        val isRefreshing: Boolean,
        val isloadingThread: Boolean,
        override val revealButton: RevealButtonState
    ) : ThreadUiState {
        val ancestors: List<TweetViewData.Concrete>
            get(): List<TweetViewData.Concrete> {
                val indexOfDetailed = statusViewData.indexOfFirst { it.isDetailed }
                return if (indexOfDetailed > 0) {
                    statusViewData.take(indexOfDetailed)
                } else {
                    emptyList()
                }
            }
        val descendants: List<TweetViewData.Concrete>
            get(): List<TweetViewData.Concrete> {
                val indexOfDetailed = statusViewData.indexOfFirst { it.isDetailed }
                return if (indexOfDetailed < statusViewData.size - 1) {
                    statusViewData.takeLast(statusViewData.size - indexOfDetailed - 1)
                } else {
                    emptyList()
                }
            }
        val detailedStatus: TweetViewData.Concrete
            get() = statusViewData.first { it.isDetailed }
    }
}

enum class RevealButtonState {
    NO_BUTTON,
    REVEAL,
    HIDE
}
