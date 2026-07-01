/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.notifications

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.TimelineUser
import site.warpnet.warpdroid.warpnet.WarpnetRepository

@HiltViewModel
class NotificationsViewModel @Inject constructor(
    private val repo: WarpnetRepository,
    private val accountManager: AccountManager,
    private val notificationHelper: NotificationHelper,
) : ViewModel() {

    data class State(
        val notifications: List<Notification> = emptyList(),
        val followRequests: List<TimelineUser> = emptyList(),
        val locked: Boolean = false,
        val loading: Boolean = true,
        val error: Throwable? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null

    init {
        reload()
    }

    fun reload() {
        loadJob?.cancel()
        loadJob = viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId
            val locked = accountManager.activeAccount?.locked ?: false
            if (userId.isNullOrEmpty()) {
                _state.update { it.copy(loading = false, locked = locked) }
                return@launch
            }
            _state.update { it.copy(loading = true, error = null, locked = locked) }
            try {
                val (notifs, _) = repo.getNotifications(userId)
                val followReqs = if (locked) repo.getFollowRequests(userId).first else emptyList()
                _state.update {
                    it.copy(
                        notifications = notifs,
                        followRequests = followReqs,
                        loading = false,
                    )
                }
                // Mark every loaded notification as read so the badge clears,
                // mirroring the Vue Notifications view behaviour.
                notifs.forEach { n ->
                    runCatching { repo.markNotificationRead(n.id) }
                }
                // The list is read now — retract anything still sitting in
                // the system tray for this account.
                accountManager.activeAccount?.let(notificationHelper::clearNotificationsForAccount)
            } catch (e: Throwable) {
                _state.update { it.copy(loading = false, error = e) }
            }
        }
    }

    fun authorize(followerId: String) {
        viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId ?: return@launch
            runCatching { repo.authorizeFollowRequest(userId, followerId) }
                .onSuccess {
                    _state.update { s ->
                        s.copy(followRequests = s.followRequests.filter { it.id != followerId })
                    }
                }
        }
    }

    fun reject(followerId: String) {
        viewModelScope.launch {
            val userId = accountManager.activeAccount?.accountId ?: return@launch
            runCatching { repo.rejectFollowRequest(userId, followerId) }
                .onSuccess {
                    _state.update { s ->
                        s.copy(followRequests = s.followRequests.filter { it.id != followerId })
                    }
                }
        }
    }
}
