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
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.NewNotificationsEvent
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
    eventHub: EventHub,
) : ViewModel() {

    data class State(
        val notifications: List<Notification> = emptyList(),
        val followRequests: List<TimelineUser> = emptyList(),
        val locked: Boolean = false,
        val loading: Boolean = true,
        val error: Throwable? = null,
        val cursor: String = "",
        val loadingMore: Boolean = false,
        val refreshing: Boolean = false,
    )

    val ownerId: String? get() = accountManager.activeAccount?.accountId

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var loadJob: Job? = null

    init {
        reload()
        viewModelScope.launch {
            eventHub.events.filterIsInstance<NewNotificationsEvent>().collect { refresh(showSpinner = false) }
        }
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
                val (notifs, cursor) = repo.getNotifications(userId)
                val followReqs = if (locked) repo.getFollowRequests(userId).first else emptyList()
                _state.update {
                    it.copy(
                        notifications = notifs,
                        followRequests = followReqs,
                        loading = false,
                        cursor = cursor,
                    )
                }
                markAllRead(notifs)
            } catch (e: Throwable) {
                _state.update { it.copy(loading = false, error = e) }
            }
        }
    }

    // Puts the newest page on top of what is loaded; older pages stay.
    fun refresh(showSpinner: Boolean) {
        val userId = accountManager.activeAccount?.accountId
        if (userId.isNullOrEmpty() || _state.value.loading) return
        viewModelScope.launch {
            if (showSpinner) _state.update { it.copy(refreshing = true) }
            val page = runCatching { repo.getNotifications(userId) }.getOrNull()
            _state.update { s ->
                if (page == null) return@update s.copy(refreshing = false)
                val fresh = page.first.mapTo(HashSet()) { it.id }
                s.copy(
                    notifications = page.first + s.notifications.filter { it.id !in fresh },
                    cursor = if (s.notifications.isEmpty()) page.second else s.cursor,
                    refreshing = false,
                )
            }
            if (page != null) markAllRead(page.first)
        }
    }

    fun loadMore() {
        val s = _state.value
        val userId = accountManager.activeAccount?.accountId
        if (userId.isNullOrEmpty() || s.loading || s.loadingMore || s.cursor.isEmpty() || s.cursor == END_CURSOR) return
        _state.update { it.copy(loadingMore = true) }
        viewModelScope.launch {
            val page = runCatching { repo.getNotifications(userId, s.cursor) }.getOrNull()
            _state.update { st ->
                if (page == null) return@update st.copy(loadingMore = false, cursor = END_CURSOR)
                val known = st.notifications.mapTo(HashSet()) { it.id }
                st.copy(
                    notifications = st.notifications + page.first.filter { it.id !in known },
                    cursor = page.second.ifEmpty { END_CURSOR },
                    loadingMore = false,
                )
            }
        }
    }

    /** The id of the chat with [otherUserId], created if there is none yet. */
    suspend fun openChat(otherUserId: String): String? {
        val ownerId = accountManager.activeAccount?.accountId ?: return null
        return runCatching { repo.createChat(ownerId, otherUserId) }.getOrNull()?.id?.ifEmpty { null }
    }

    private suspend fun markAllRead(notifs: List<Notification>) {
        // Mark everything read node-side so the badge clears — including
        // unread items beyond the loaded page. Falls back to per-item
        // marking when the paired node predates the read-all route.
        runCatching { repo.markAllNotificationsRead() }
            .onFailure {
                notifs.forEach { n ->
                    runCatching { repo.markNotificationRead(n.id) }
                }
            }
        // The list is read now — retract anything still sitting in the
        // system tray for this account.
        accountManager.activeAccount?.let(notificationHelper::clearNotificationsForAccount)
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

    private companion object {
        const val END_CURSOR = "end"
    }
}
