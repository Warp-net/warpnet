/* Copyright 2019 Joel Pyska
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

package site.warpnet.warpdroid.components.report

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.Pager
import androidx.paging.PagingConfig
import androidx.paging.cachedIn
import androidx.paging.map
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.appstore.BlockEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.MuteEvent
import site.warpnet.warpdroid.components.report.adapter.StatusesPagingSource
import site.warpnet.warpdroid.components.report.model.TweetViewState
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Instance
import site.warpnet.warpdroid.entity.Relationship
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.Error
import site.warpnet.warpdroid.util.Loading
import site.warpnet.warpdroid.util.Resource
import site.warpnet.warpdroid.util.Success
import site.warpnet.warpdroid.util.isHttpNotFound
import site.warpnet.warpdroid.util.toViewData
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
    private val warpnetApi: WarpnetApi,
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
        pagingSourceFactory = { StatusesPagingSource(accountId, warpnetApi) }
    ).flow
        .map { pagingData ->
            /* TODO: refactor reports to use the isShowingContent / isExpanded / isCollapsed attributes from TweetViewData.Concrete
             instead of TweetViewState */
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
    val statusViewState = TweetViewState()

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
            warpnetApi.getInstanceRules().fold(
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
            warpnetApi.relationships(ids).fold(
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
                warpnetApi.unmuteAccount(accountId)
            } else {
                warpnetApi.muteAccount(accountId)
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
                warpnetApi.unblockAccount(accountId)
            } else {
                warpnetApi.blockAccount(accountId)
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
            warpnetApi.report(
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

    fun setStatusChecked(status: Tweet, checked: Boolean) {
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
