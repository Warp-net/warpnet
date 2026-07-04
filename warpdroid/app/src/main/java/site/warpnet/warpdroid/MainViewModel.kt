/* Copyright 2024 Warpdroid Contributors
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

package site.warpnet.warpdroid

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.fold
import site.warpnet.warpdroid.appstore.ConversationsLoadingEvent
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.NewNotificationsEvent
import site.warpnet.warpdroid.appstore.NotificationsLoadingEvent
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Emoji
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.util.ShareShortcutHelper
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import retrofit2.HttpException
import timber.log.Timber

@HiltViewModel
class MainViewModel @Inject constructor(
    private val api: WarpnetApi,
    private val eventHub: EventHub,
    private val accountManager: AccountManager,
    private val shareShortcutHelper: ShareShortcutHelper,
    private val notificationHelper: NotificationHelper,
) : ViewModel() {

    private val activeAccount = accountManager.activeAccount!!

    val accounts: StateFlow<List<AccountViewData>> = accountManager.accountsFlow
        .map { accounts ->
            accounts.map { account ->
                AccountViewData(
                    id = account.id,
                    domain = account.domain,
                    username = account.username,
                    displayName = account.displayName,
                    profilePictureUrl = account.profilePictureUrl,
                    profileHeaderUrl = account.profileHeaderUrl,
                    emojis = account.emojis
                )
            }
        }
        .stateIn(viewModelScope, SharingStarted.Lazily, emptyList())

    val tabs: StateFlow<List<TabData>> = accountManager.activeAccount(viewModelScope)
        .mapNotNull { account -> account?.tabPreferences }
        .stateIn(viewModelScope, SharingStarted.Eagerly, activeAccount.tabPreferences)

    val showDirectMessagesBadge: StateFlow<Boolean> = accountManager.activeAccount(viewModelScope)
        .map { account -> account?.hasDirectMessageBadge == true }
        .stateIn(viewModelScope, SharingStarted.Eagerly, false)

    private val _unauthorized: MutableStateFlow<Boolean> = MutableStateFlow(false)
    val unauthorized: StateFlow<Boolean> = _unauthorized.asStateFlow()

    init {
        loadAccountData()
        collectEvents()
    }

    private fun loadAccountData() {
        viewModelScope.launch {
            api.accountVerifyCredentials().fold(
                { userInfo ->
                    accountManager.updateAccount(activeAccount, userInfo)

                    shareShortcutHelper.updateShortcuts()
                },
                { throwable ->
                    Timber.tag(TAG).e(throwable, "Failed to fetch user info.")

                    if (throwable is HttpException && throwable.code() == 401) {
                        _unauthorized.value = true
                    }
                }
            )
        }
    }

    private fun collectEvents() {
        viewModelScope.launch {
            eventHub.events.collect { event ->
                when (event) {
                    is NewNotificationsEvent -> {
                        if (event.accountId == activeAccount.accountId) {
                            val hasDirectMessageNotification =
                                event.notifications.any {
                                    it.type == Notification.Type.Mention &&
                                        it.status?.visibility == Tweet.Visibility.DIRECT
                                }

                            if (hasDirectMessageNotification) {
                                accountManager.updateAccount(activeAccount) { copy(hasDirectMessageBadge = true) }
                            }
                        }
                    }
                    is NotificationsLoadingEvent -> {
                        if (event.accountId == activeAccount.accountId) {
                            accountManager.updateAccount(activeAccount) { copy(hasDirectMessageBadge = false) }
                        }
                    }
                    is ConversationsLoadingEvent -> {
                        if (event.accountId == activeAccount.accountId) {
                            accountManager.updateAccount(activeAccount) { copy(hasDirectMessageBadge = false) }
                        }
                    }
                }
            }
        }
    }

    fun dismissDirectMessagesBadge() {
        viewModelScope.launch {
            accountManager.updateAccount(activeAccount) { copy(hasDirectMessageBadge = false) }
        }
    }

    fun setupNotifications(activity: MainActivity) {
        // TODO this is only called on full app (re) start; so changes in-between (push distributor uninstalled/subscription changed, or
        //   notifications fully disabled) will get unnoticed; and also an app restart cannot be easily triggered by the user.

        // TODO it's quite odd to separate channel creation (for an account) from the "is enabled by channels" question below
        notificationHelper.createNotificationChannelsForAccount(activeAccount)

        if (notificationHelper.areNotificationsEnabledBySystem()) {
            viewModelScope.launch {
                notificationHelper.setupNotifications(activity)
            }
        } else {
            viewModelScope.launch {
                notificationHelper.disableAllNotifications()
            }
        }
    }

    companion object {
        private const val TAG = "MainViewModel"
    }
}

data class AccountViewData(
    val id: Long,
    val domain: String,
    val username: String,
    val displayName: String,
    val profilePictureUrl: String,
    val profileHeaderUrl: String,
    val emojis: List<Emoji>
) {
    val fullName: String
        get() = "@$username@$domain"
}
