/* Copyright 2019 Joel Pyska
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

package com.keylesspalace.tusky.components.report

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import androidx.paging.map
import at.connyduck.calladapter.networkresult.fold
import com.keylesspalace.tusky.appstore.BlockEvent
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.MuteEvent
import com.keylesspalace.tusky.components.report.adapter.StatusesPagingSource
import com.keylesspalace.tusky.components.report.model.StatusViewState
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Instance
import com.keylesspalace.tusky.entity.Relationship
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.network.MastodonApi
import com.keylesspalace.tusky.util.Error
import com.keylesspalace.tusky.util.Loading
import com.keylesspalace.tusky.util.Resource
import com.keylesspalace.tusky.util.Success
import com.keylesspalace.tusky.util.isHttpNotFound
import com.keylesspalace.tusky.util.toViewData
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

@HiltViewModel(assistedFactory = ReportViewModel.Factory::class)
class ReportViewModel @AssistedInject constructor(
    private val mastodonApi: MastodonApi,
    private val eventHub: EventHub,
    @Assisted("accountId") val accountId: String,
    @Assisted("userName") val userName: String,
    @Assisted("statusId") private val statusId: String?
) : ViewModel() {

    private val _navigation: MutableStateFlow<Int?> = MutableStateFlow(0)
    val navigation: StateFlow<Int?> = _navigation.asStateFlow()

    private val _reportCategory: MutableStateFlow<ReportCategory?> = MutableStateFlow(null)
    val reportCategory: StateFlow<ReportCategory?> = _reportCategory.asStateFlow()

    val showRules: StateFlow<Boolean> = _reportCategory.map { selectedCategory ->
        selectedCategory == ReportCategory.VIOLATION
    }.stateIn(viewModelScope, SharingStarted.Eagerly, false)

    private val _rules: MutableStateFlow<Resource<List<Instance.Rule>>> = MutableStateFlow(Loading(null))
    val rules: StateFlow<Resource<List<Instance.Rule>>?> = _rules.asStateFlow()

    private val _selectedRules: MutableStateFlow<Set<String>> = MutableStateFlow(emptySet())
    val selectedRules: StateFlow<Set<String>> = _selectedRules.asStateFlow()

    private val _muteState: MutableStateFlow<Resource<Boolean>?> = MutableStateFlow(null)
    val muteState: StateFlow<Resource<Boolean>?> = _muteState.asStateFlow()

    private val _blockState: MutableStateFlow<Resource<Boolean>?> = MutableStateFlow(null)
    val blockState: StateFlow<Resource<Boolean>?> = _blockState.asStateFlow()

    private val _reportingState: MutableStateFlow<Resource<Boolean>?> = MutableStateFlow(null)
    var reportingState: StateFlow<Resource<Boolean>?> = _reportingState.asStateFlow()

    private val _checkUrl: MutableStateFlow<String?> = MutableStateFlow(null)
    val checkUrl: StateFlow<String?> = _checkUrl.asStateFlow()

    val statusesFlow = Pager(
        initialKey = statusId,
        config = PagingConfig(
            pageSize = 20,
            initialLoadSize = 20
        ),
        pagingSourceFactory = { StatusesPagingSource(accountId, mastodonApi) }
    ).flow
        .map { pagingData ->
            /* TODO: refactor reports to use the isShowingContent / isExpanded / isCollapsed attributes from StatusViewData.Concrete
             instead of StatusViewState */
            pagingData.map { status ->
                status.toViewData(
                    isShowingContent = false,
                    isExpanded = false,
                    isCollapsed = false,
                    filterKind = Filter.Kind.PUBLIC,
                    filterActive = true,
                    isQuoteShowingContent = false,
                    isQuoteExpanded = false,
                    isQuoteCollapsed = false,
                    isQuoteShown = false
                )
            }
        }
        .cachedIn(viewModelScope)

    private val selectedIds = HashSet<String>()
    val statusViewState = StatusViewState()

    var reportNote: String = ""
    var isRemoteNotify = false

    val isRemoteAccount: Boolean = userName.contains('@')
    val remoteServer: String? = if (isRemoteAccount) {
        userName.substring(userName.indexOf('@') + 1)
    } else {
        null
    }

    init {
        statusId?.let {
            selectedIds.add(it)
        }
        loadInstanceRules()
        obtainRelationship()
    }

    fun forwardFrom(screen: Screen) {
        when (screen) {
            Screen.Category -> _navigation.value = 1
            Screen.Rules -> _navigation.value = 2
            Screen.Statuses -> _navigation.value = if (showRules.value) 3 else 2
            Screen.Note -> _navigation.value = if (showRules.value) 4 else 3
            Screen.Done -> _navigation.value = null
        }
    }

    fun backFrom(screen: Screen) {
        when (screen) {
            Screen.Category -> _navigation.value = null
            Screen.Rules -> _navigation.value = 0
            Screen.Statuses -> _navigation.value = if (showRules.value) 1 else 0
            Screen.Note -> _navigation.value = if (showRules.value) 2 else 1
            Screen.Done -> { /* not allowed */ }
        }
    }

    fun selectReportCategory(category: ReportCategory) {
        _reportCategory.value = category
    }

    fun toggleRule(ruleId: String) {
        _selectedRules.update { selectedRules ->
            if (selectedRules.contains(ruleId)) {
                selectedRules - ruleId
            } else {
                selectedRules + ruleId
            }
        }
    }

    fun loadInstanceRules() {
        viewModelScope.launch {
            mastodonApi.getInstanceRules().fold(
                { rules ->
                    _rules.value = Success(rules)
                },
                { e ->
                    if (e.isHttpNotFound()) {
                        // instance does not support rules
                        _rules.value = Success(emptyList())
                    } else {
                        _rules.value = Error(cause = e)
                    }
                }
            )
        }
    }

    private fun obtainRelationship() {
        val ids = listOf(accountId)
        _muteState.value = Loading()
        _blockState.value = Loading()
        viewModelScope.launch {
            mastodonApi.relationships(ids).fold(
                { data ->
                    updateRelationship(data.getOrNull(0))
                },
                {
                    updateRelationship(null)
                }
            )
        }
    }

    private fun updateRelationship(relationship: Relationship?) {
        if (relationship != null) {
            _muteState.value = Success(relationship.muting)
            _blockState.value = Success(relationship.blocking)
        } else {
            _muteState.value = Error(false)
            _blockState.value = Error(false)
        }
    }

    fun toggleMute() {
        val alreadyMuted = _muteState.value?.data == true
        viewModelScope.launch {
            if (alreadyMuted) {
                mastodonApi.unmuteAccount(accountId)
            } else {
                mastodonApi.muteAccount(accountId)
            }.fold(
                { relationship ->
                    val muting = relationship.muting
                    _muteState.value = Success(muting)
                    if (muting) {
                        eventHub.dispatch(MuteEvent(accountId))
                    }
                },
                { t ->
                    _muteState.value = Error(false, t.message)
                }
            )
        }

        _muteState.value = Loading()
    }

    fun toggleBlock() {
        val alreadyBlocked = _blockState.value?.data == true
        viewModelScope.launch {
            if (alreadyBlocked) {
                mastodonApi.unblockAccount(accountId)
            } else {
                mastodonApi.blockAccount(accountId)
            }.fold({ relationship ->
                val blocking = relationship.blocking
                _blockState.value = Success(blocking)
                if (blocking) {
                    eventHub.dispatch(BlockEvent(accountId))
                }
            }, { t ->
                _blockState.value = Error(false, t.message)
            })
        }
        _blockState.value = Loading()
    }

    fun doReport() {
        _reportingState.value = Loading()
        viewModelScope.launch {
            mastodonApi.report(
                accountId,
                statusIds = selectedIds,
                comment = reportNote,
                forward = if (isRemoteAccount) isRemoteNotify else null,
                category = _reportCategory.value?.serverName,
                ruleIds = selectedRules.value
            )
                .fold({
                    _reportingState.value = Success(true)
                }, { error ->
                    _reportingState.value = Error(cause = error)
                })
        }
    }

    fun checkClickedUrl(url: String?) {
        _checkUrl.value = url
    }

    fun urlChecked() {
        _checkUrl.value = null
    }

    fun setStatusChecked(status: Status, checked: Boolean) {
        if (checked) {
            selectedIds.add(status.id)
        } else {
            selectedIds.remove(status.id)
        }
    }

    fun isStatusChecked(id: String): Boolean {
        return selectedIds.contains(id)
    }

    @AssistedFactory
    interface Factory {
        fun create(
            @Assisted("accountId") accountId: String,
            @Assisted("userName") userName: String,
            @Assisted("statusId") statusId: String?
        ): ReportViewModel
    }

    enum class ReportCategory(val serverName: String) {
        SPAM("spam"),
        LEGAL("legal"),
        VIOLATION("violation"),
        OTHER("other")
    }

    enum class Screen {
        Category,
        Rules,
        Statuses,
        Note,
        Done
    }
}
