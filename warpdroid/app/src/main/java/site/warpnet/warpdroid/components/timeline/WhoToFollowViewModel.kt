/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Backs the "who to follow" carousel shown inside the home timeline. Loads the
 * fat node's account recommendations once, and tracks which ones the user has
 * followed so their button can flip to "Following" without a reload.
 */
package site.warpnet.warpdroid.components.timeline

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.warpnet.WarpnetRepository

@HiltViewModel
class WhoToFollowViewModel @Inject constructor(
    private val repo: WarpnetRepository,
    private val accountManager: AccountManager,
) : ViewModel() {

    data class State(
        val accounts: List<TimelineUser> = emptyList(),
        val followed: Set<String> = emptySet(),
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loaded = false
    private var reloadJob: Job? = null

    /** Load recommendations the first time the home timeline shows them. */
    fun loadOnce() {
        if (loaded) return
        loaded = true
        reload()
    }

    fun reload() {
        val userId = accountManager.activeAccount?.accountId ?: return
        reloadJob?.cancel() // a newer refresh supersedes any in-flight one
        reloadJob = viewModelScope.launch {
            val accounts = try {
                repo.whoToFollow(userId).first
            } catch (e: CancellationException) {
                throw e
            } catch (e: Throwable) {
                return@launch // keep previously loaded suggestions on error
            }
            _state.update { it.copy(accounts = accounts) }
        }
    }

    fun follow(account: TimelineUser) {
        val userId = accountManager.activeAccount?.accountId ?: return
        if (account.id in _state.value.followed) return
        _state.update { it.copy(followed = it.followed + account.id) } // optimistic
        viewModelScope.launch {
            try {
                repo.followAccount(followerId = userId, followeeId = account.id)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Throwable) {
                _state.update { s -> s.copy(followed = s.followed - account.id) } // revert
            }
        }
    }
}
