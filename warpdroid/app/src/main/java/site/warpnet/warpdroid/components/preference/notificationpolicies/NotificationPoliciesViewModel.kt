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

package site.warpnet.warpdroid.components.preference.notificationpolicies

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import at.connyduck.calladapter.networkresult.onFailure
import site.warpnet.warpdroid.usecase.NotificationPolicyState
import site.warpnet.warpdroid.usecase.NotificationPolicyUsecase
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch

@HiltViewModel
class NotificationPoliciesViewModel @Inject constructor(
    private val usecase: NotificationPolicyUsecase
) : ViewModel() {

    val state: StateFlow<NotificationPolicyState> = usecase.state

    private val _error = MutableSharedFlow<Throwable>(
        replay = 0,
        extraBufferCapacity = 1,
        onBufferOverflow = BufferOverflow.DROP_OLDEST
    )
    val error: SharedFlow<Throwable> = _error.asSharedFlow()

    init {
        loadPolicy()
    }

    fun loadPolicy() {
        viewModelScope.launch {
            usecase.getNotificationPolicy()
        }
    }

    fun updatePolicy(
        forNotFollowing: String? = null,
        forNotFollowers: String? = null,
        forNewAccounts: String? = null,
        forPrivateMentions: String? = null,
        forLimitedAccounts: String? = null
    ) {
        viewModelScope.launch {
            usecase.updatePolicy(
                forNotFollowing = forNotFollowing,
                forNotFollowers = forNotFollowers,
                forNewAccounts = forNewAccounts,
                forPrivateMentions = forPrivateMentions,
                forLimitedAccounts = forLimitedAccounts
            ).onFailure { error ->
                Log.w(TAG, "failed to update notifications policy", error)
                _error.emit(error)
            }
        }
    }

    companion object {
        private const val TAG = "NotificationPoliciesViewModel"
    }
}
