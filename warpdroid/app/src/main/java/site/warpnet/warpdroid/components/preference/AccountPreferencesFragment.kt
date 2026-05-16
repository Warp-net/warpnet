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
import android.graphics.drawable.Drawable
import android.os.Build
import android.os.Bundle
import android.util.Log
import androidx.lifecycle.lifecycleScope
import androidx.preference.ListPreference
import at.connyduck.calladapter.networkresult.fold
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.BuildConfig
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.appstore.PreferenceChangedEvent
import site.warpnet.warpdroid.components.accountlist.AccountListActivity
import site.warpnet.warpdroid.components.filters.FiltersActivity
import site.warpnet.warpdroid.components.followedtags.FollowedTagsActivity
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.entity.Account
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.network.WarpnetApi
import site.warpnet.warpdroid.settings.AccountPreferenceDataStore
import site.warpnet.warpdroid.settings.DefaultReplyVisibility
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.settings.QuotePolicy
import site.warpnet.warpdroid.settings.listPreference
import site.warpnet.warpdroid.settings.makePreferenceScreen
import site.warpnet.warpdroid.settings.preference
import site.warpnet.warpdroid.settings.preferenceCategory
import site.warpnet.warpdroid.settings.switchPreference
import site.warpnet.warpdroid.util.getInitialLanguages
import site.warpnet.warpdroid.util.getLocaleList
import site.warpnet.warpdroid.util.getWarpdroidDisplayName
import site.warpnet.warpdroid.util.icon
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.launch

@AndroidEntryPoint
class AccountPreferencesFragment : BasePreferencesFragment() {
    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var warpnetApi: WarpnetApi

    @Inject
    lateinit var eventHub: EventHub

    @Inject
    lateinit var accountPreferenceDataStore: AccountPreferenceDataStore

    override fun onCreatePreferences(savedInstanceState: Bundle?, rootKey: String?) {
        val context = requireContext()
        makePreferenceScreen {
            preference {
                setTitle(R.string.pref_title_edit_notification_settings)
                icon = icon(R.drawable.ic_notifications_24dp)
                setOnPreferenceClickListener {
                    openNotificationSystemPrefs()
                    true
                }
            }

            // "Customize tabs" is removed in Warpdroid — the two tabs are fixed.

            preference {
                setTitle(R.string.title_followed_hashtags)
                icon = icon(R.drawable.ic_tag_24dp)
                setOnPreferenceClickListener {
                    val intent = Intent(context, FollowedTagsActivity::class.java)
                    activity?.startActivityWithSlideInAnimation(intent)
                    true
                }
            }

            preference {
                setTitle(R.string.action_view_mutes)
                icon = icon(R.drawable.ic_volume_off_24dp)
                setOnPreferenceClickListener {
                    val intent = Intent(context, AccountListActivity::class.java)
                    intent.putExtra("type", AccountListActivity.Type.MUTES)
                    activity?.startActivityWithSlideInAnimation(intent)
                    true
                }
            }

            preference {
                setTitle(R.string.action_view_blocks)
                icon = icon(R.drawable.ic_block_24dp)
                setOnPreferenceClickListener {
                    val intent = Intent(context, AccountListActivity::class.java)
                    intent.putExtra("type", AccountListActivity.Type.BLOCKS)
                    activity?.startActivityWithSlideInAnimation(intent)
                    true
                }
            }

            preference {
                setTitle(R.string.pref_title_timeline_filters)
                icon = icon(R.drawable.ic_filter_alt_24dp)
                setOnPreferenceClickListener {
                    launchFilterActivity()
                    true
                }
            }

            preferenceCategory(R.string.pref_publishing) {
                listPreference {
                    setTitle(R.string.pref_default_post_privacy)
                    setEntries(R.array.post_privacy_names)
                    setEntryValues(R.array.post_privacy_values)
                    key = PrefKeys.DEFAULT_POST_PRIVACY
                    setSummaryProvider { entry }
                    val visibility = accountManager.activeAccount?.defaultPostPrivacy ?: Tweet.Visibility.PUBLIC
                    value = visibility.stringValue
                    icon = getIconForVisibility(visibility)
                    isPersistent = false // its saved to the account and shouldn't be in shared preferences
                    setOnPreferenceChangeListener { _, newValue ->
                        icon = getIconForVisibility(Tweet.Visibility.fromStringValue(newValue as String))
                        if (accountManager.activeAccount?.defaultReplyPrivacy == DefaultReplyVisibility.MATCH_DEFAULT_POST_VISIBILITY) {
                            findPreference<ListPreference>(PrefKeys.DEFAULT_REPLY_PRIVACY)?.icon = icon
                        }
                        syncWithServer(visibility = newValue)
                        true
                    }
                }

                val activeAccount = accountManager.activeAccount
                if (activeAccount != null) {
                    listPreference {
                        setTitle(R.string.pref_default_reply_privacy)
                        setEntries(R.array.reply_privacy_names)
                        setEntryValues(R.array.reply_privacy_values)
                        key = PrefKeys.DEFAULT_REPLY_PRIVACY
                        setSummaryProvider { entry }
                        val visibility = activeAccount.defaultReplyPrivacy
                        value = visibility.stringValue
                        icon = getIconForVisibility(visibility.toVisibilityOr(activeAccount.defaultPostPrivacy))
                        isPersistent = false // its saved to the account and shouldn't be in shared preferences
                        setOnPreferenceChangeListener { _, newValue ->
                            val newVisibility = DefaultReplyVisibility.fromStringValue(newValue as String)

                            icon = getIconForVisibility(newVisibility.toVisibilityOr(activeAccount.defaultPostPrivacy))

                            viewLifecycleOwner.lifecycleScope.launch {
                                accountManager.updateAccount(activeAccount) { copy(defaultReplyPrivacy = newVisibility) }
                                eventHub.dispatch(PreferenceChangedEvent(key))
                            }
                            true
                        }
                    }
                    preference {
                        setSummary(R.string.pref_default_reply_privacy_explanation)
                        shouldDisableView = false
                        isEnabled = false
                    }
                }

                listPreference {
                    setTitle(R.string.pref_default_quote_policy)
                    setEntries(R.array.quote_policy_names)
                    setEntryValues(R.array.quote_policy_values)
                    key = PrefKeys.DEFAULT_QUOTE_POLICY
                    setSummaryProvider { entry }
                    val policy = accountManager.activeAccount?.defaultQuotePolicy ?: QuotePolicy.FOLLOWERS
                    value = policy.text
                    icon = getIconForQuotePolicy(policy)
                    isPersistent = false // its saved to the account and shouldn't be in shared preferences
                    setOnPreferenceChangeListener { _, newValue ->
                        icon = getIconForQuotePolicy(QuotePolicy.forValue(newValue as String) ?: QuotePolicy.FOLLOWERS)
                        syncWithServer(quotePolicy = newValue)
                        true
                    }
                }

                listPreference {
                    val locales =
                        getLocaleList(getInitialLanguages(null, accountManager.activeAccount))
                    setTitle(R.string.pref_default_post_language)
                    // Explicitly add "System default" to the start of the list
                    entries = (
                        listOf(context.getString(R.string.system_default)) + locales.map {
                            it.getWarpdroidDisplayName(context)
                        }
                        ).toTypedArray()
                    entryValues = (listOf("") + locales.map { it.language }).toTypedArray()
                    key = PrefKeys.DEFAULT_POST_LANGUAGE
                    icon = icon(R.drawable.ic_translate_24dp)
                    value = accountManager.activeAccount?.defaultPostLanguage.orEmpty()
                    isPersistent = false // This will be entirely server-driven
                    setSummaryProvider { entry }

                    setOnPreferenceChangeListener { _, newValue ->
                        syncWithServer(language = (newValue as String))
                        true
                    }
                }

                switchPreference {
                    setTitle(R.string.pref_default_media_sensitivity)
                    icon = icon(R.drawable.ic_visibility_24dp)
                    key = PrefKeys.DEFAULT_MEDIA_SENSITIVITY
                    val sensitivity = accountManager.activeAccount?.defaultMediaSensitivity == true
                    setDefaultValue(sensitivity)
                    icon = getIconForSensitivity(sensitivity)
                    setOnPreferenceChangeListener { _, newValue ->
                        icon = getIconForSensitivity(newValue as Boolean)
                        syncWithServer(sensitive = newValue)
                        true
                    }
                }
            }

            preferenceCategory(R.string.pref_title_timelines) {
                // TODO having no activeAccount in this fragment does not really make sense, enforce it?
                //   All other locations here make it optional, however.

                switchPreference {
                    key = PrefKeys.MEDIA_PREVIEW_ENABLED
                    setTitle(R.string.pref_title_show_media_preview)
                    preferenceDataStore = accountPreferenceDataStore
                }

                switchPreference {
                    key = PrefKeys.ALWAYS_SHOW_SENSITIVE_MEDIA
                    setTitle(R.string.pref_title_alway_show_sensitive_media)
                    preferenceDataStore = accountPreferenceDataStore
                }

                switchPreference {
                    key = PrefKeys.ALWAYS_OPEN_SPOILER
                    setTitle(R.string.pref_title_alway_open_spoiler)
                    preferenceDataStore = accountPreferenceDataStore
                }
            }
            preferenceCategory(R.string.pref_title_per_timeline_preferences) {
                preference {
                    setTitle(R.string.pref_title_post_tabs)
                    fragment = TabFilterPreferencesFragment::class.qualifiedName
                }
            }
        }
    }

    override fun onResume() {
        super.onResume()
        requireActivity().setTitle(R.string.action_view_account_preferences)
    }

    private fun openNotificationSystemPrefs() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val intent = Intent()
            intent.action = "android.settings.APP_NOTIFICATION_SETTINGS"
            intent.putExtra("android.provider.extra.APP_PACKAGE", BuildConfig.APPLICATION_ID)
            startActivity(intent)
        } else {
            activity?.let {
                val intent = PreferencesActivity.newIntent(
                    it,
                    PreferencesActivity.NOTIFICATION_PREFERENCES
                )
                it.startActivityWithSlideInAnimation(intent)
            }
        }
    }

    private fun syncWithServer(
        visibility: String? = null,
        sensitive: Boolean? = null,
        language: String? = null,
        quotePolicy: String? = null,
    ) {
        // TODO these could also be "datastore backed" preferences (a ServerPreferenceDataStore); follow-up of issue #3204

        viewLifecycleOwner.lifecycleScope.launch {
            warpnetApi.accountUpdateSource(visibility, sensitive, language, quotePolicy)
                .fold({ account: Account ->
                    accountManager.activeAccount?.let {
                        accountManager.updateAccount(it) {
                            copy(
                                defaultPostPrivacy = account.source?.privacy
                                    ?: Tweet.Visibility.PUBLIC,
                                defaultMediaSensitivity = account.source?.sensitive == true,
                                defaultPostLanguage = language.orEmpty(),
                                defaultQuotePolicy = quotePolicy?.let { QuotePolicy.forValue(it) } ?: QuotePolicy.FOLLOWERS,
                            )
                        }
                    }
                }, { t ->
                    Log.e("AccountPreferences", "failed updating settings on server", t)
                    showErrorSnackbar(visibility, sensitive)
                })
        }
    }

    private fun showErrorSnackbar(visibility: String?, sensitive: Boolean?) {
        view?.let { view ->
            Snackbar.make(view, R.string.pref_failed_to_sync, Snackbar.LENGTH_LONG)
                .setAction(R.string.action_retry) { syncWithServer(visibility, sensitive) }
                .show()
        }
    }

    private fun getIconForVisibility(visibility: Tweet.Visibility): Drawable? {
        val iconRes = when (visibility) {
            Tweet.Visibility.PRIVATE -> R.drawable.ic_lock_24dp
            Tweet.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
            Tweet.Visibility.DIRECT -> R.drawable.ic_mail_24dp
            else -> R.drawable.ic_public_24dp
        }
        return icon(iconRes)
    }

    private fun getIconForQuotePolicy(policy: QuotePolicy) = when (policy) {
        QuotePolicy.NOBODY -> R.drawable.ic_lock_24dp
        QuotePolicy.PUBLIC -> R.drawable.ic_public_24dp
        else -> R.drawable.ic_group_24dp
    }.let { icon(it) }

    private fun getIconForSensitivity(sensitive: Boolean): Drawable? {
        return if (sensitive) {
            icon(R.drawable.ic_visibility_off_24dp)
        } else {
            icon(R.drawable.ic_visibility_24dp)
        }
    }

    private fun launchFilterActivity() {
        val intent = Intent(context, FiltersActivity::class.java)
        (activity as? BaseActivity)?.startActivityWithSlideInAnimation(intent)
    }

    companion object {
        fun newInstance() = AccountPreferencesFragment()
    }
}
