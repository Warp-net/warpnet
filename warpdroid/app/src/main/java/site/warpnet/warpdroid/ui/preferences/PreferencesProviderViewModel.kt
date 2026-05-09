package site.warpnet.warpdroid.ui.preferences

/* Copyright 2025 Warpdroid Contributors
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

import android.content.SharedPreferences
import androidx.compose.ui.unit.sp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import site.warpnet.warpdroid.components.preference.PreferencesFragment.ReadingOrder
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.settings.AppTheme
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.settings.PrefKeys.APP_THEME
import site.warpnet.warpdroid.util.getNonNullString
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.stateIn

@HiltViewModel
class PreferencesProviderViewModel @Inject constructor(
    private val sharedPreferences: SharedPreferences,
    accountManager: AccountManager,
) : ViewModel() {

    val activeAccount = accountManager.activeAccount(viewModelScope)

    val warpdroidPreferences: StateFlow<WarpdroidPreferences> = callbackFlow {

        val listener = SharedPreferences.OnSharedPreferenceChangeListener { sharedPreferences, key ->
            trySend(sharedPreferences.toWarpdroidPreferences())
        }

        sharedPreferences.registerOnSharedPreferenceChangeListener(listener)

        awaitClose { sharedPreferences.unregisterOnSharedPreferenceChangeListener(listener) }
    }.stateIn(viewModelScope, SharingStarted.Lazily, sharedPreferences.toWarpdroidPreferences())

    private fun SharedPreferences.toWarpdroidPreferences(): WarpdroidPreferences {
        val warpdroidTheme = this.getNonNullString(APP_THEME, AppTheme.DEFAULT.value)

        val statusTextStyles = when (this.getString(PrefKeys.STATUS_TEXT_SIZE, "medium")) {
            "smallest" -> StatusTextStyles(
                small = textStyle(fontSize = 10.sp),
                medium = textStyle(fontSize = 12.sp),
                large = textStyle(fontSize = 14.sp)
            )
            "small" -> StatusTextStyles(
                small = textStyle(fontSize = 12.sp),
                medium = textStyle(fontSize = 14.sp),
                large = textStyle(fontSize = 16.sp)
            )
            "medium" -> StatusTextStyles(
                small = textStyle(fontSize = 14.sp),
                medium = textStyle(fontSize = 16.sp),
                large = textStyle(fontSize = 18.sp)
            )
            "large" -> StatusTextStyles(
                small = textStyle(fontSize = 16.sp),
                medium = textStyle(fontSize = 18.sp),
                large = textStyle(fontSize = 20.sp)
            )
            "largest" -> StatusTextStyles(
                small = textStyle(fontSize = 18.sp),
                medium = textStyle(fontSize = 20.sp),
                large = textStyle(fontSize = 22.sp)
            )
            else -> StatusTextStyles(
                small = textStyle(fontSize = 14.sp),
                medium = textStyle(fontSize = 16.sp),
                large = textStyle(fontSize = 18.sp)
            )
        }

        return WarpdroidPreferences(
            theme = AppTheme.of(warpdroidTheme),
            statusTextStyles = statusTextStyles,
            useBlurhash = this.getBoolean(PrefKeys.USE_BLURHASH, true),
            showBotBadge = this.getBoolean(PrefKeys.SHOW_BOT_OVERLAY, true),
            animateCustomEmojis = this.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false),
            animateAvatars = this.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false),
            useAbsoluteTime = this.getBoolean(PrefKeys.ABSOLUTE_TIME_VIEW, false),
            showStatsInline = this.getBoolean(PrefKeys.SHOW_STATS_INLINE, true),
            showLinkPreviews = this.getBoolean(PrefKeys.SHOW_CARDS_IN_TIMELINES, false),
            readingOrder = ReadingOrder.from(
                this.getString(PrefKeys.READING_ORDER, null)
            ),
            wellbeing = WellbeingSettings(
                limitTimelineNotifications = this.getBoolean(PrefKeys.WELLBEING_LIMITED_NOTIFICATIONS, false),
                hideQuantitativeStatsOnPosts = this.getBoolean(PrefKeys.WELLBEING_HIDE_STATS_POSTS, false),
                hideQuantitativeStatsOnProfiles = this.getBoolean(PrefKeys.WELLBEING_HIDE_STATS_PROFILE, false)
            )
        )
    }
}
