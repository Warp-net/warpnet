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

package com.keylesspalace.tusky.components.timeline.viewmodel

import android.content.SharedPreferences
import androidx.lifecycle.viewModelScope
import androidx.paging.PagingData
import at.connyduck.calladapter.networkresult.NetworkResult
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.FilterUpdatedEvent
import com.keylesspalace.tusky.appstore.PollShowResultsEvent
import com.keylesspalace.tusky.appstore.PreferenceChangedEvent
import com.keylesspalace.tusky.components.preference.PreferencesFragment.ReadingOrder
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.network.MastodonApi
import com.keylesspalace.tusky.settings.PrefKeys
import com.keylesspalace.tusky.viewdata.StatusViewData
import com.keylesspalace.tusky.viewmodel.StatusActionsViewModel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch

abstract class TimelineViewModel(
    api: MastodonApi,
    private val eventHub: EventHub,
    val accountManager: AccountManager,
    private val sharedPreferences: SharedPreferences
) : StatusActionsViewModel(api, eventHub) {

    val activeAccountFlow = accountManager.activeAccount(viewModelScope)
    protected val accountId: Long = activeAccountFlow.value!!.id

    abstract val statuses: Flow<PagingData<StatusViewData>>

    var kind: Kind = Kind.HOME
        private set
    var id: String? = null
        private set
    var tags: List<String> = emptyList()
        private set

    protected var alwaysShowSensitiveMedia = false
    private var alwaysOpenSpoilers = false
    private var filterRemoveReplies = false
    private var filterRemoveReblogs = false
    private var filterRemoveSelfReblogs = false
    protected var readingOrder: ReadingOrder = ReadingOrder.OLDEST_FIRST

    fun init(kind: Kind, id: String?, tags: List<String>) {
        this.kind = kind
        this.id = id
        this.tags = tags

        val activeAccount = activeAccountFlow.value!!

        if (kind == Kind.HOME) {
            // Note the variable is "true if filter" but the underlying preference/settings text is "true if show"
            filterRemoveReplies = !activeAccount.isShowHomeReplies
            filterRemoveReblogs = !activeAccount.isShowHomeBoosts
            filterRemoveSelfReblogs = !activeAccount.isShowHomeSelfBoosts
        }
        readingOrder = ReadingOrder.from(sharedPreferences.getString(PrefKeys.READING_ORDER, null))

        this.alwaysShowSensitiveMedia = activeAccount.alwaysShowSensitiveMedia
        this.alwaysOpenSpoilers = activeAccount.alwaysOpenSpoiler

        viewModelScope.launch {
            eventHub.events
                .collect { event ->
                    when (event) {
                        is PreferenceChangedEvent -> {
                            onPreferenceChanged(event.preferenceKey)
                        }
                        is FilterUpdatedEvent -> {
                            if (filterContextMatchesKind(this@TimelineViewModel.kind, event.filterContext)) {
                                invalidate()
                            }
                        }
                    }
                }
        }
    }

    fun showPollResults(status: StatusViewData.Concrete) = viewModelScope.launch {
        eventHub.dispatch(PollShowResultsEvent(status.actionableId))
    }

    abstract fun changeExpanded(expanded: Boolean, status: StatusViewData.Concrete)

    abstract fun changeContentShowing(isShowing: Boolean, status: StatusViewData.Concrete)

    abstract fun changeContentCollapsed(isCollapsed: Boolean, status: StatusViewData.Concrete)

    abstract fun removeStatusWithId(id: String)

    abstract fun loadMore(placeholderId: String)

    abstract fun fullReload()

    abstract fun changeFilter(filtered: Boolean, status: StatusViewData.Concrete)

    abstract fun showQuote(status: StatusViewData.Concrete)

    /** Triggered when currently displayed data must be reloaded. */
    protected abstract suspend fun invalidate()

    protected fun shouldHideStatus(statusViewData: StatusViewData): Boolean {
        val concrete = statusViewData.asStatusOrNull() ?: return false
        val status = concrete.status
        return if (
            (status.isReply && filterRemoveReplies) ||
            (status.reblog != null && filterRemoveReblogs) ||
            (status.account.id == status.reblog?.account?.id && filterRemoveSelfReblogs)
        ) {
            true
        } else if (status.actionableStatus.account.id == activeAccountFlow.value?.accountId) {
            // Mastodon filters don't apply for own posts
            false
        } else {
            concrete.isFilterHide
        }
    }

    private fun onPreferenceChanged(key: String) {
        activeAccountFlow.value?.let { activeAccount ->
            when (key) {
                PrefKeys.TAB_FILTER_HOME_REPLIES -> {
                    val filter = !activeAccount.isShowHomeReplies
                    val oldRemoveReplies = filterRemoveReplies
                    filterRemoveReplies = kind == Kind.HOME && !filter
                    if (oldRemoveReplies != filterRemoveReplies) {
                        fullReload()
                    }
                }

                PrefKeys.TAB_FILTER_HOME_BOOSTS -> {
                    val filter = !activeAccount.isShowHomeBoosts
                    val oldRemoveReblogs = filterRemoveReblogs
                    filterRemoveReblogs = kind == Kind.HOME && !filter
                    if (oldRemoveReblogs != filterRemoveReblogs) {
                        fullReload()
                    }
                }

                PrefKeys.TAB_SHOW_HOME_SELF_BOOSTS -> {
                    val filter = !activeAccount.isShowHomeSelfBoosts
                    val oldRemoveSelfReblogs = filterRemoveSelfReblogs
                    filterRemoveSelfReblogs = kind == Kind.HOME && !filter
                    if (oldRemoveSelfReblogs != filterRemoveSelfReblogs) {
                        fullReload()
                    }
                }

                PrefKeys.ALWAYS_SHOW_SENSITIVE_MEDIA -> {
                    // it is ok if only newly loaded statuses are affected, no need to fully refresh
                    alwaysShowSensitiveMedia = activeAccount.alwaysShowSensitiveMedia
                }

                PrefKeys.READING_ORDER -> {
                    readingOrder = ReadingOrder.from(sharedPreferences.getString(PrefKeys.READING_ORDER, null))
                }
            }
        }
    }

    abstract suspend fun translate(status: StatusViewData.Concrete): NetworkResult<Unit>
    abstract fun untranslate(status: StatusViewData.Concrete)
    abstract fun saveHomeTimelinePosition(firstVisibleIndex: Int, firstVisibleOffset: Int)

    companion object {
        private const val TAG = "TimelineVM"
        internal const val LOAD_AT_ONCE = 30

        fun filterContextMatchesKind(kind: Kind, filterContext: List<Filter.Kind>): Boolean =
            filterContext.contains(kind.toFilterKind())
    }

    /** Types of timelines we can display
     * @param isOrdered If true, the timeline is ordered by status id, which allows doing custom pagination.
     * If false, pagination can only be done via link headers.
     */
    enum class Kind(val isOrdered: Boolean) {
        HOME(isOrdered = true),
        PUBLIC_LOCAL(isOrdered = true),
        PUBLIC_FEDERATED(isOrdered = true),
        TAG(isOrdered = true),
        USER(isOrdered = true),
        USER_PINNED(isOrdered = false),
        USER_WITH_REPLIES(isOrdered = true),
        FAVOURITES(isOrdered = false),
        LIST(isOrdered = true),
        BOOKMARKS(isOrdered = false),
        PUBLIC_TRENDING_STATUSES(isOrdered = false),
        QUOTES(isOrdered = true);

        fun toFilterKind(): Filter.Kind {
            return when (valueOf(name)) {
                HOME, LIST -> Filter.Kind.HOME
                PUBLIC_FEDERATED, PUBLIC_LOCAL, TAG, FAVOURITES, PUBLIC_TRENDING_STATUSES -> Filter.Kind.PUBLIC
                USER, USER_WITH_REPLIES, USER_PINNED -> Filter.Kind.ACCOUNT
                else -> Filter.Kind.PUBLIC
            }
        }
    }
}
