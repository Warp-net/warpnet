/* Copyright 2018 Conny Duck
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

package site.warpnet.warpdroid.components.preference

import android.content.Intent
import android.content.SharedPreferences
import android.os.Bundle
import androidx.annotation.DrawableRes
import androidx.lifecycle.lifecycleScope
import androidx.preference.Preference
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.logviewer.LogViewerActivity
import site.warpnet.warpdroid.components.systemnotifications.NotificationChannelData
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.settings.AppTheme
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.settings.emojiPreference
import site.warpnet.warpdroid.settings.listPreference
import site.warpnet.warpdroid.settings.makePreferenceScreen
import site.warpnet.warpdroid.settings.preference
import site.warpnet.warpdroid.settings.preferenceCategory
import site.warpnet.warpdroid.settings.sliderPreference
import site.warpnet.warpdroid.settings.switchPreference
import site.warpnet.warpdroid.util.LocaleManager
import site.warpnet.warpdroid.util.icon
import dagger.hilt.android.AndroidEntryPoint
import de.c1710.filemojicompat_ui.views.picker.preference.EmojiPickerPreference
import javax.inject.Inject
import kotlinx.coroutines.launch

@AndroidEntryPoint
class PreferencesFragment : BasePreferencesFragment() {

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var localeManager: LocaleManager

    @Inject
    lateinit var sharedPrefs: SharedPreferences

    enum class ReadingOrder {
        /** User scrolls up, reading statuses oldest to newest.
         *  "Load more" gaps fill with the oldest posts first. */
        OLDEST_FIRST,

        /** User scrolls down, reading statuses newest to oldest. Default behaviour.
         *  "Load more" gaps fill with the newest posts first. */
        NEWEST_FIRST;

        companion object {
            fun from(s: String?): ReadingOrder {
                s ?: return NEWEST_FIRST

                return try {
                    valueOf(s.uppercase())
                } catch (_: Throwable) {
                    NEWEST_FIRST
                }
            }
        }
    }

    override fun onCreatePreferences(savedInstanceState: Bundle?, rootKey: String?) {
        makePreferenceScreen {
            preferenceCategory(R.string.pref_title_appearance_settings) {
                listPreference {
                    setDefaultValue(AppTheme.DEFAULT.value)
                    setEntries(R.array.app_theme_names)
                    entryValues = AppTheme.stringValues()
                    key = PrefKeys.APP_THEME
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_title_app_theme)
                    icon = icon(R.drawable.ic_palette_24dp)
                }

                emojiPreference(requireActivity()) {
                    setTitle(R.string.emoji_style)
                    icon = icon(R.drawable.ic_mood_24dp)
                }

                listPreference {
                    setDefaultValue("default")
                    setEntries(R.array.language_entries)
                    setEntryValues(R.array.language_values)
                    key = PrefKeys.LANGUAGE + "_" // deliberately not the actual key, the real handling happens in LocaleManager
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_title_language)
                    icon = icon(R.drawable.ic_translate_24dp)
                    preferenceDataStore = localeManager
                }

                sliderPreference {
                    key = PrefKeys.UI_TEXT_SCALE_RATIO
                    setDefaultValue(100F)
                    valueTo = 150F
                    valueFrom = 50F
                    stepSize = 5F
                    setTitle(R.string.pref_ui_text_size)
                    format = "%.0f%%"
                    decrementIcon = icon(R.drawable.ic_zoom_out_24dp)
                    incrementIcon = icon(R.drawable.ic_zoom_in_24dp)
                    icon = icon(R.drawable.ic_format_size_24dp)
                }

                listPreference {
                    setDefaultValue("medium")
                    setEntries(R.array.post_text_size_names)
                    setEntryValues(R.array.post_text_size_values)
                    key = PrefKeys.STATUS_TEXT_SIZE
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_post_text_size)
                    icon = icon(R.drawable.ic_format_size_24dp)
                }

                listPreference {
                    setDefaultValue(ReadingOrder.NEWEST_FIRST.name)
                    setEntries(R.array.reading_order_names)
                    setEntryValues(R.array.reading_order_values)
                    key = PrefKeys.READING_ORDER
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_title_reading_order)
                    icon = icon(R.drawable.ic_sort_24dp)
                }

                listPreference {
                    setDefaultValue("top")
                    setEntries(R.array.pref_main_nav_position_options)
                    setEntryValues(R.array.pref_main_nav_position_values)
                    key = PrefKeys.MAIN_NAV_POSITION
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_main_nav_position)
                    icon = icon(
                        navigationPositionIcon(
                            sharedPrefs.getString(PrefKeys.MAIN_NAV_POSITION, "top").orEmpty()
                        )
                    )
                    setOnPreferenceChangeListener { _, newValue ->
                        icon = icon(navigationPositionIcon(newValue.toString()))
                        true
                    }
                }

                listPreference {
                    setDefaultValue("disambiguate")
                    setEntries(R.array.pref_show_self_username_names)
                    setEntryValues(R.array.pref_show_self_username_values)
                    key = PrefKeys.SHOW_SELF_USERNAME
                    setSummaryProvider { entry }
                    setTitle(R.string.pref_title_show_self_username)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.HIDE_TOP_TOOLBAR
                    setTitle(R.string.pref_title_hide_top_toolbar)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.SHOW_NOTIFICATIONS_FILTER
                    setTitle(R.string.pref_title_show_notifications_filter)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.ABSOLUTE_TIME_VIEW
                    setTitle(R.string.pref_title_absolute_time)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.SHOW_BOT_OVERLAY
                    setTitle(R.string.pref_title_bot_overlay)
                    icon = icon(R.drawable.ic_bot_24dp)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.ANIMATE_GIF_AVATARS
                    setTitle(R.string.pref_title_animate_gif_avatars)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.ANIMATE_CUSTOM_EMOJIS
                    setTitle(R.string.pref_title_animate_custom_emojis)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.USE_BLURHASH
                    setTitle(R.string.pref_title_gradient_for_media)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.SHOW_CARDS_IN_TIMELINES
                    setTitle(R.string.pref_title_show_cards_in_timelines)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.CONFIRM_RETWEETS
                    setTitle(R.string.pref_title_confirm_retweets)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.CONFIRM_LIKES
                    setTitle(R.string.pref_title_confirm_likes)
                }

                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.CONFIRM_FOLLOWS
                    setTitle(R.string.pref_title_confirm_follows)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.ENABLE_SWIPE_FOR_TABS
                    setTitle(R.string.pref_title_enable_swipe_for_tabs)
                }

                switchPreference {
                    setDefaultValue(true)
                    key = PrefKeys.SHOW_STATS_INLINE
                    setTitle(R.string.pref_title_show_stat_inline)
                }
            }

            preferenceCategory(R.string.pref_title_browser_settings) {
                switchPreference {
                    setDefaultValue(false)
                    key = PrefKeys.CUSTOM_TABS
                    setTitle(R.string.pref_title_custom_tabs)
                }
            }

            preferenceCategory(R.string.pref_title_wellbeing_mode) {
                switchPreference {
                    title = getString(R.string.limit_notifications)
                    setDefaultValue(false)
                    key = PrefKeys.WELLBEING_LIMITED_NOTIFICATIONS
                    setOnPreferenceChangeListener { _, value ->
                        for (account in accountManager.accounts) {
                            val notificationFilter = account.notificationsFilter.toMutableSet()

                            if (value == true) {
                                notificationFilter.add(NotificationChannelData.LIKE)
                                notificationFilter.add(NotificationChannelData.FOLLOW)
                                notificationFilter.add(NotificationChannelData.RETWEET)
                            } else {
                                notificationFilter.remove(NotificationChannelData.LIKE)
                                notificationFilter.remove(NotificationChannelData.FOLLOW)
                                notificationFilter.remove(NotificationChannelData.RETWEET)
                            }

                            lifecycleScope.launch {
                                accountManager.updateAccount(account) { copy(notificationsFilter = notificationFilter) }
                            }
                        }
                        true
                    }
                }

                switchPreference {
                    title = getString(R.string.wellbeing_hide_stats_posts)
                    setDefaultValue(false)
                    key = PrefKeys.WELLBEING_HIDE_STATS_POSTS
                }

                switchPreference {
                    title = getString(R.string.wellbeing_hide_stats_profile)
                    setDefaultValue(false)
                    key = PrefKeys.WELLBEING_HIDE_STATS_PROFILE
                }
            }

            preferenceCategory(R.string.pref_title_proxy_settings) {
                preference {
                    setTitle(R.string.pref_title_http_proxy_settings)
                    fragment = ProxyPreferencesFragment::class.qualifiedName
                    summaryProvider = ProxyPreferencesFragment.SummaryProvider
                }
            }

            preferenceCategory(R.string.pref_title_developer_tools) {
                preference {
                    setTitle(R.string.pref_title_logs)
                    icon = icon(R.drawable.ic_list_alt_24dp)
                    setOnPreferenceClickListener {
                        startActivity(Intent(requireContext(), LogViewerActivity::class.java))
                        true
                    }
                }
            }
        }
    }

    @DrawableRes private fun navigationPositionIcon(position: String): Int {
        return if (position == "bottom") {
            R.drawable.ic_bottom_navigation_24dp
        } else {
            R.drawable.ic_bottom_navigation_24dp_mirrored
        }
    }

    override fun onResume() {
        super.onResume()
        requireActivity().setTitle(R.string.action_view_preferences)
    }

    override fun onDisplayPreferenceDialog(preference: Preference) {
        if (!EmojiPickerPreference.onDisplayPreferenceDialog(this, preference)) {
            super.onDisplayPreferenceDialog(preference)
        }
    }

    companion object {
        fun newInstance(): PreferencesFragment {
            return PreferencesFragment()
        }
    }
}
