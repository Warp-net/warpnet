/* Copyright 2020 Warpdroid Contributors
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

import android.Manifest
import android.annotation.SuppressLint
import android.app.NotificationManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.DialogInterface
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.ColorStateList
import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.drawable.Animatable
import android.graphics.drawable.Drawable
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.text.TextUtils
import android.util.Log
import android.view.KeyEvent
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.MenuItem.SHOW_AS_ACTION_NEVER
import android.view.View
import android.view.ViewGroup
import android.view.ViewGroup.LayoutParams
import android.widget.ImageView
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.appcompat.R as appcompatR
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.content.res.AppCompatResources
import androidx.coordinatorlayout.widget.CoordinatorLayout
import androidx.core.app.ActivityCompat
import androidx.core.content.edit
import androidx.core.content.pm.ShortcutManagerCompat
import androidx.core.graphics.drawable.toDrawable
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.core.view.MenuProvider
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.forEach
import androidx.core.view.isVisible
import androidx.core.view.updateLayoutParams
import androidx.core.view.updatePadding
import androidx.lifecycle.lifecycleScope
import androidx.viewpager2.widget.MarginPageTransformer
import com.bumptech.glide.Glide
import com.bumptech.glide.load.resource.bitmap.RoundedCorners
import com.bumptech.glide.request.target.CustomTarget
import androidx.core.content.ContextCompat
import com.bumptech.glide.request.target.FixedSizeDrawable
import com.bumptech.glide.request.transition.Transition
import com.google.android.material.R as materialR
import com.google.android.material.color.MaterialColors
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.tabs.TabLayout
import com.google.android.material.tabs.TabLayout.OnTabSelectedListener
import com.google.android.material.tabs.TabLayoutMediator
import site.warpnet.warpdroid.appstore.EventHub
import site.warpnet.warpdroid.components.account.AccountActivity
import site.warpnet.warpdroid.components.accountlist.AccountListActivity
import site.warpnet.warpdroid.components.chats.ChatsActivity
import site.warpnet.warpdroid.components.notifications.NotificationsActivity
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.compose.ComposeActivity.Companion.canHandleMimeType
import site.warpnet.warpdroid.components.pairing.PairedNodeStore
import site.warpnet.warpdroid.components.preference.PreferencesActivity
import site.warpnet.warpdroid.components.search.SearchActivity
import site.warpnet.warpdroid.components.systemnotifications.NotificationHelper
import site.warpnet.warpdroid.databinding.ActivityMainBinding
import site.warpnet.warpdroid.db.DraftsAlert
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Notification
import site.warpnet.warpdroid.interfaces.AccountSelectionListener
import site.warpnet.warpdroid.interfaces.ActionButtonActivity
import site.warpnet.warpdroid.interfaces.ReselectableFragment
import site.warpnet.warpdroid.pager.MainPagerAdapter
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.usecase.LogoutUsecase
import site.warpnet.warpdroid.util.getParcelableExtraCompat
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.loadHeader
import site.warpnet.warpdroid.util.reduceSwipeSensitivity
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.viewBinding
import com.mikepenz.materialdrawer.model.AbstractDrawerItem
import com.mikepenz.materialdrawer.model.DividerDrawerItem
import com.mikepenz.materialdrawer.model.PrimaryDrawerItem
import com.mikepenz.materialdrawer.model.ProfileDrawerItem
import com.mikepenz.materialdrawer.model.ProfileSettingDrawerItem
import com.mikepenz.materialdrawer.model.SecondaryDrawerItem
import com.mikepenz.materialdrawer.model.interfaces.IProfile
import com.mikepenz.materialdrawer.model.interfaces.descriptionRes
import com.mikepenz.materialdrawer.model.interfaces.descriptionText
import com.mikepenz.materialdrawer.model.interfaces.iconRes
import com.mikepenz.materialdrawer.model.interfaces.nameRes
import com.mikepenz.materialdrawer.model.interfaces.nameText
import com.mikepenz.materialdrawer.util.AbstractDrawerImageLoader
import com.mikepenz.materialdrawer.util.DrawerImageLoader
import com.mikepenz.materialdrawer.util.addItems
import com.mikepenz.materialdrawer.util.addItemsAtPosition
import com.mikepenz.materialdrawer.util.updateBadge
import com.mikepenz.materialdrawer.widget.AccountHeaderView
import dagger.hilt.android.AndroidEntryPoint
import dagger.hilt.android.migration.OptionalInject
import javax.inject.Inject
import kotlinx.coroutines.launch

@OptionalInject
@AndroidEntryPoint
class MainActivity : BottomSheetActivity(), ActionButtonActivity, MenuProvider {

    @Inject
    lateinit var eventHub: EventHub

    @Inject
    lateinit var notificationHelper: NotificationHelper

    @Inject
    lateinit var logoutUsecase: LogoutUsecase

    @Inject
    lateinit var draftsAlert: DraftsAlert

    @Inject
    lateinit var pairedNodeStore: PairedNodeStore

    @Inject
    lateinit var connectionMonitor: site.warpnet.transport.ConnectionMonitor

    private val viewModel: MainViewModel by viewModels()

    private val binding by viewBinding(ActivityMainBinding::inflate)

    private lateinit var activeAccount: AccountEntity

    private lateinit var header: AccountHeaderView

    private var onTabSelectedListener: OnTabSelectedListener? = null

    /** Mediate between binding.viewPager and the chosen tab layout */
    private var tabLayoutMediator: TabLayoutMediator? = null

    /** Adapter for the different timeline tabs */
    private lateinit var tabAdapter: MainPagerAdapter

    private var directMessageTab: TabLayout.Tab? = null

    private val onBackPressedCallback = object : OnBackPressedCallback(false) {
        override fun handleOnBackPressed() {
            binding.viewPager.currentItem = 0
        }
    }

    private val requestNotificationPermissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { isGranted ->
            if (isGranted) {
                viewModel.setupNotifications(this)
            }
        }

    private val requestIgnoreBatteryOptimizationsLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { }

    @SuppressLint("RestrictedApi")
    override fun onCreate(savedInstanceState: Bundle?) {
        // Newer Android versions don't need to install the compat Splash Screen
        // and it can cause theming bugs.
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) {
            installSplashScreen()
        }
        super.onCreate(savedInstanceState)

        // make sure MainActivity doesn't hide other activities when launcher icon is clicked again
        if (!isTaskRoot &&
            intent.hasCategory(Intent.CATEGORY_LAUNCHER) &&
            intent.action == Intent.ACTION_MAIN
        ) {
            finish()
            return
        }

        // If no fat-node pairing yet, bounce to the QR scanner. The returning
        // PairingActivity clears the task, so this branch only fires once per
        // cold install (or after "Forget this node").
        val paired = pairedNodeStore.load()
        if (paired == null) {
            startActivity(Intent(this, site.warpnet.warpdroid.components.pairing.PairingActivity::class.java))
            finish()
            return
        }

        // AccountManager is in-memory only, so the stub account is always
        // accountId="me" at process start. WarpnetApi resolves the
        // authenticated user via accountManager.activeAccount?.accountId, so
        // without this sync every "me"-scoped backend call (notifications,
        // createStatus, deleteStatus, like/unlike, follow/unfollow, …) would
        // send the literal string "me" instead of the paired user id.
        if (accountManager.activeAccount?.accountId != paired.userId) {
            accountManager.updateActiveAccount { copy(accountId = paired.userId) }
        }

        activeAccount = accountManager.activeAccount ?: return

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ActivityCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestNotificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
        } else {
            viewModel.setupNotifications(this)
        }

        // check for savedInstanceState in order to not handle intent events more than once
        if (intent != null && savedInstanceState == null) {
            handleIntent(intent, activeAccount)
            if (isFinishing) {
                // handleIntent() finished this activity and started a new one - no need to continue initialization
                return
            }
        }

        requestIgnoreBatteryOptimizations()

        setContentView(binding.root)

        val bottomBarHeight = if (preferences.getString(PrefKeys.MAIN_NAV_POSITION, "top") == "bottom") {
            resources.getDimensionPixelSize(R.dimen.bottomAppBarHeight)
        } else {
            0
        }

        val fabMargin = resources.getDimensionPixelSize(R.dimen.fabMargin)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM) {
            ViewCompat.setOnApplyWindowInsetsListener(binding.viewPager) { _, insets ->
                val systemBarsInsets = insets.getInsets(systemBars())
                val bottomInsets = systemBarsInsets.bottom

                binding.composeButton.updateLayoutParams<CoordinatorLayout.LayoutParams> {
                    bottomMargin = bottomBarHeight + fabMargin + bottomInsets
                }
                binding.mainDrawer.recyclerView.updatePadding(bottom = bottomInsets)

                if (preferences.getString(PrefKeys.MAIN_NAV_POSITION, "top") == "top") {
                    insets
                } else {
                    binding.viewPager.updatePadding(bottom = bottomBarHeight + bottomInsets)

                    /* BottomAppBar could handle size and insets automatically, but then it gets quite large,
                       so we do it like this instead */
                    binding.bottomNav.updateLayoutParams<ViewGroup.MarginLayoutParams> {
                        height = bottomBarHeight + bottomInsets
                    }
                    binding.bottomTabLayout.updateLayoutParams<ViewGroup.MarginLayoutParams> {
                        bottomMargin = bottomInsets
                    }
                    insets.inset(0, 0, 0, bottomInsets)
                }
            }
        } else {
            // don't draw a status bar, the DrawerLayout and the MaterialDrawerLayout have their own
            // on Vanilla Ice Cream (API 35) and up there is no status bar color because of edge-to-edge mode
            @Suppress("DEPRECATION")
            window.statusBarColor = Color.TRANSPARENT

            binding.composeButton.updateLayoutParams<CoordinatorLayout.LayoutParams> {
                bottomMargin = bottomBarHeight + fabMargin
            }
            binding.viewPager.updatePadding(bottom = bottomBarHeight)
        }

        binding.composeButton.setOnClickListener {
            val composeIntent = Intent(applicationContext, ComposeActivity::class.java)
            startActivity(composeIntent)
        }

        // Determine which of the three toolbars should be the supportActionBar (which hosts
        // the options menu).
        val hideTopToolbar = preferences.getBoolean(PrefKeys.HIDE_TOP_TOOLBAR, false)
        if (hideTopToolbar) {
            when (preferences.getString(PrefKeys.MAIN_NAV_POSITION, "top")) {
                "top" -> setSupportActionBar(binding.topNav)
                "bottom" -> setSupportActionBar(binding.bottomNav)
            }
            // this is a bit hacky, but when the mainToolbar is GONE, the toolbar size gets messed up for some reason
            binding.mainToolbar.layoutParams.height = 0
            binding.mainToolbar.visibility = View.INVISIBLE
            // There's not enough space in the top/bottom bars to show the title as well.
            supportActionBar?.setDisplayShowTitleEnabled(false)
        } else {
            setSupportActionBar(binding.mainToolbar)
            binding.mainToolbar.layoutParams.height = LayoutParams.WRAP_CONTENT
            binding.mainToolbar.show()
        }

        addMenuProvider(this)

        binding.viewPager.reduceSwipeSensitivity()

        setupDrawer(
            savedInstanceState,
            addSearchButton = hideTopToolbar,
        )

        lifecycleScope.launch {
            viewModel.accounts.collect(::updateProfiles)
        }

        // Initialise the tab adapter and set to viewpager. Fragments appear to be leaked if the
        // adapter changes over the life of the viewPager (the adapter, not its contents), so set
        // the initial list of tabs to empty, and set the full list later in setupTabs(). See
        // https://github.com/tuskyapp/Tusky/issues/3251 for details.
        tabAdapter = MainPagerAdapter(emptyList(), this@MainActivity)
        binding.viewPager.adapter = tabAdapter
        binding.viewPager.offscreenPageLimit = 2

        lifecycleScope.launch {
            viewModel.tabs.collect(::setupTabs)
        }
        // Warpdroid: no Notifications tab to switch to.

        lifecycleScope.launch {
            viewModel.showDirectMessagesBadge.collect { showBadge ->
                updateDirectMessageBadge(showBadge)
            }
        }

        lifecycleScope.launch {
            viewModel.unauthorized.collect(::showUnauthorizedWarning)
        }

        lifecycleScope.launch {
            connectionMonitor.linkState.collect { state ->
                val color = ContextCompat.getColor(
                    this@MainActivity,
                    if (state.isUp) R.color.warpdroid_green else R.color.warpdroid_red,
                )
                binding.connectionIndicator.imageTintList = ColorStateList.valueOf(color)
                binding.connectionIndicator.contentDescription = getString(
                    if (state.isUp) {
                        R.string.connection_indicator_connected
                    } else {
                        R.string.connection_indicator_disconnected
                    },
                )
            }
        }

        onBackPressedDispatcher.addCallback(this@MainActivity, onBackPressedCallback)

        // "Post failed" dialog should display in this activity
        draftsAlert.observeInContext(this@MainActivity, true)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        // Warpdroid: no Notifications tab; ignore the "show notifications" hint.
        handleIntent(intent, activeAccount)
    }

    override fun onDestroy() {
        super.onDestroy()
    }

    /** Handle an incoming Intent,
     * @returns true when the intent is coming from an notification and the interface should show the notification tab.
     */
    private fun handleIntent(intent: Intent, activeAccount: AccountEntity): Boolean {
        val notificationId = intent.getIntExtra(NOTIFICATION_ID, -1)
        if (notificationId != -1) {
            // opened from a notification action, cancel the notification
            val notificationManager = getSystemService(
                NOTIFICATION_SERVICE
            ) as NotificationManager
            notificationManager.cancel(intent.getStringExtra(NOTIFICATION_TAG), notificationId)
        }

        /** there are two possibilities the accountId can be passed to MainActivity:
         * - from our code as Long Intent Extra WARPDROID_ACCOUNT_ID
         * - from share shortcuts as String 'android.intent.extra.shortcut.ID'
         */
        var warpdroidAccountId = intent.getLongExtra(WARPDROID_ACCOUNT_ID, -1)
        if (warpdroidAccountId == -1L) {
            val accountIdString = intent.getStringExtra(ShortcutManagerCompat.EXTRA_SHORTCUT_ID)
            if (accountIdString != null) {
                warpdroidAccountId = accountIdString.toLong()
            }
        }
        val accountRequested = warpdroidAccountId != -1L
        if (accountRequested && warpdroidAccountId != activeAccount.id) {
            changeAccount(warpdroidAccountId, intent)
            return false
        }

        if (canHandleMimeType(intent.type) || intent.hasExtra(COMPOSE_OPTIONS)) {
            // Sharing to Warpdroid from an external app
            if (accountRequested) {
                // The correct account is already active
                forwardToComposeActivity(intent)
            } else {
                // No account was provided, show the chooser
                showAccountChooserDialog(
                    getString(R.string.action_share_as),
                    true,
                    object : AccountSelectionListener {
                        override fun onAccountSelected(account: AccountEntity) {
                            val requestedId = account.id
                            if (requestedId == activeAccount.id) {
                                // The correct account is already active
                                forwardToComposeActivity(intent)
                            } else {
                                // A different account was requested, restart the activity
                                intent.putExtra(WARPDROID_ACCOUNT_ID, requestedId)
                                changeAccount(requestedId, intent)
                            }
                        }
                    }
                )
            }
        } else if (accountRequested && intent.hasExtra(NOTIFICATION_TYPE)) {
            // user clicked a notification, show follow requests for type FOLLOW_REQUEST,
            // otherwise show notification tab
            if (intent.getStringExtra(NOTIFICATION_TYPE) == Notification.Type.FollowRequest.name) {
                val accountListIntent = AccountListActivity.newIntent(
                    this,
                    AccountListActivity.Type.FOLLOW_REQUESTS
                )
                startActivityWithSlideInAnimation(accountListIntent)
            } else {
                return true
            }
        }
        return false
    }

    private fun updateDirectMessageBadge(showBadge: Boolean) {
        directMessageTab?.badge?.isVisible = showBadge
    }

    private fun showUnauthorizedWarning(unauthorized: Boolean) {
        if (unauthorized) {
            MaterialAlertDialogBuilder(this)
                .setTitle(R.string.account_disconnected_title)
                .setMessage(getString(R.string.account_disconnected_message, activeAccount.fullName))
                .setNeutralButton(R.string.action_remove_account) { _, _ ->
                    logout(true)
                }
                .show()
            // No relogin path — Warpdroid is session-less.
        }
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.activity_main, menu)
    }

    override fun onPrepareMenu(menu: Menu) {
        super.onPrepareMenu(menu)

        // If the main toolbar is hidden then there's no space in the top/bottomNav to show
        // the menu items as icons, so forceably disable them
        if (!binding.mainToolbar.isVisible) {
            menu.forEach {
                it.setShowAsAction(
                    SHOW_AS_ACTION_NEVER
                )
            }
        }
    }

    override fun onMenuItemSelected(item: MenuItem): Boolean {
        return when (item.itemId) {
            R.id.action_search -> {
                startActivity(SearchActivity.getIntent(this@MainActivity))
                true
            }
            else -> super.onOptionsItemSelected(item)
        }
    }

    override fun dispatchKeyEvent(event: KeyEvent): Boolean {
        // Allow software back press to be properly dispatched to drawer layout
        val handled = when (event.action) {
            KeyEvent.ACTION_DOWN -> binding.mainDrawerLayout.onKeyDown(event.keyCode, event)
            KeyEvent.ACTION_UP -> binding.mainDrawerLayout.onKeyUp(event.keyCode, event)
            else -> false
        }
        return handled || super.dispatchKeyEvent(event)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        when (keyCode) {
            KeyEvent.KEYCODE_MENU -> {
                if (binding.mainDrawerLayout.isOpen) {
                    binding.mainDrawerLayout.close()
                } else {
                    binding.mainDrawerLayout.open()
                }
                return true
            }
            KeyEvent.KEYCODE_SEARCH -> {
                startActivityWithSlideInAnimation(SearchActivity.getIntent(this))
                return true
            }
        }
        if (event.isCtrlPressed || event.isShiftPressed) {
            // FIXME: blackberry keyONE raises SHIFT key event even CTRL IS PRESSED
            when (keyCode) {
                KeyEvent.KEYCODE_N -> {
                    // open compose activity by pressing SHIFT + N (or CTRL + N)
                    val composeIntent = Intent(applicationContext, ComposeActivity::class.java)
                    startActivity(composeIntent)
                    return true
                }
            }
        }
        return super.onKeyDown(keyCode, event)
    }

    public override fun onPostCreate(savedInstanceState: Bundle?) {
        super.onPostCreate(savedInstanceState)

        if (intent != null) {
            val redirectUrl = intent.getStringExtra(REDIRECT_URL)
            if (redirectUrl != null) {
                viewUrl(redirectUrl, PostLookupFallbackBehavior.DISPLAY_ERROR)
            }
        }
    }

    private fun forwardToComposeActivity(intent: Intent) {
        val composeOptions =
            intent.getParcelableExtraCompat<ComposeActivity.ComposeOptions>(COMPOSE_OPTIONS)
        val composeIntent = if (composeOptions != null) {
            ComposeActivity.newIntent(this, composeOptions)
        } else {
            Intent(this, ComposeActivity::class.java).apply {
                action = intent.action
                type = intent.type
                putExtras(intent)
            }
        }
        startActivity(composeIntent)
    }

    private fun setupDrawer(
        savedInstanceState: Bundle?,
        addSearchButton: Boolean,
    ) {
        val drawerOpenClickListener = View.OnClickListener { binding.mainDrawerLayout.open() }

        binding.mainToolbar.setNavigationOnClickListener(drawerOpenClickListener)
        binding.topNav.setNavigationOnClickListener(drawerOpenClickListener)
        binding.bottomNav.setNavigationOnClickListener(drawerOpenClickListener)

        header = AccountHeaderView(this).apply {
            headerBackgroundScaleType = ImageView.ScaleType.CENTER_CROP
            currentHiddenInList = true
            onAccountHeaderListener = { _: View?, profile: IProfile, current: Boolean -> handleProfileClick(profile, current) }
            addProfile(
                ProfileSettingDrawerItem().apply {
                    identifier = DRAWER_ITEM_ADD_ACCOUNT
                    nameRes = R.string.add_account_name
                    descriptionRes = R.string.add_account_description
                    iconRes = R.drawable.ic_add_24dp
                    isIconTinted = true
                },
                0
            )
            attachToSliderView(binding.mainDrawer)
            dividerBelowHeader = false
            closeDrawerOnProfileListClick = true
        }

        header.currentProfileName.maxLines = 1
        header.currentProfileName.ellipsize = TextUtils.TruncateAt.END

        header.accountHeaderBackground.setColorFilter(getColor(R.color.headerBackgroundFilter))
        header.accountHeaderBackground.setBackgroundColor(
            MaterialColors.getColor(header, R.attr.colorBackgroundAccent)
        )
        val animateAvatars = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false)

        DrawerImageLoader.init(object : AbstractDrawerImageLoader() {
            override fun set(imageView: ImageView, uri: Uri, placeholder: Drawable, tag: String?) {
                // Warpnet has no HTTP URLs for headers/avatars in some
                // states (newly-paired profile, etc.); skip the Glide
                // request entirely so logcat isn't spammed with
                // "Load failed for []" for an empty Uri. Cancel any
                // in-flight request on this recycled ImageView first so
                // a stale callback can't overwrite the placeholder.
                if (uri.toString().isBlank()) {
                    Glide.with(imageView).clear(imageView)
                    imageView.setImageDrawable(placeholder)
                    return
                }
                if (animateAvatars) {
                    Glide.with(imageView)
                        .load(uri)
                        .placeholder(placeholder)
                        .into(imageView)
                } else {
                    Glide.with(imageView)
                        .asBitmap()
                        .load(uri)
                        .placeholder(placeholder)
                        .into(imageView)
                }
            }

            override fun cancel(imageView: ImageView) {
                // nothing to do, Glide already handles cancellation automatically
            }

            override fun placeholder(ctx: Context, tag: String?): Drawable {
                if (tag == DrawerImageLoader.Tags.PROFILE.name || tag == DrawerImageLoader.Tags.PROFILE_DRAWER_ITEM.name) {
                    return AppCompatResources.getDrawable(ctx, R.drawable.avatar_default)!!
                }

                return super.placeholder(ctx, tag)
            }
        })

        binding.mainDrawer.apply {
            refreshMainDrawerItems(
                addSearchButton = addSearchButton,
            )
            setSavedInstance(savedInstanceState)
        }
    }

    private fun refreshMainDrawerItems(
        addSearchButton: Boolean,
    ) {
        binding.mainDrawer.apply {
            itemAdapter.clear()
            tintStatusBar = true
            addItems(
                primaryDrawerItem {
                    nameRes = R.string.action_view_profile
                    iconRes = R.drawable.ic_person_24dp
                    onClick = {
                        val ownId = accountManager.activeAccount?.accountId
                        if (!ownId.isNullOrEmpty()) {
                            startActivityWithSlideInAnimation(AccountActivity.newIntent(context, ownId))
                        }
                    }
                },
                primaryDrawerItem {
                    nameRes = R.string.title_notifications
                    iconRes = R.drawable.ic_notifications_24dp
                    onClick = {
                        startActivityWithSlideInAnimation(NotificationsActivity.newIntent(context))
                    }
                },
                primaryDrawerItem {
                    nameRes = R.string.title_direct_messages
                    iconRes = R.drawable.ic_mail_24dp
                    onClick = {
                        startActivityWithSlideInAnimation(ChatsActivity.newIntent(context))
                    }
                },
                primaryDrawerItem {
                    nameRes = R.string.action_view_bookmarks
                    iconRes = R.drawable.ic_bookmark_24dp
                    onClick = {
                        val intent = TweetListActivity.newBookmarksIntent(context)
                        startActivityWithSlideInAnimation(intent)
                    }
                },
                primaryDrawerItem {
                    nameRes = R.string.title_likes
                    iconRes = R.drawable.ic_star_24dp
                    onClick = {
                        val intent = TweetListActivity.newLikesIntent(context)
                        startActivityWithSlideInAnimation(intent)
                    }
                },
                DividerDrawerItem(),
                secondaryDrawerItem {
                    nameRes = R.string.action_view_account_preferences
                    iconRes = R.drawable.ic_manage_accounts_24dp
                    onClick = {
                        val intent = PreferencesActivity.newIntent(context, PreferencesActivity.ACCOUNT_PREFERENCES)
                        startActivityWithSlideInAnimation(intent)
                    }
                },
                secondaryDrawerItem {
                    nameRes = R.string.action_view_preferences
                    iconRes = R.drawable.ic_settings_24dp
                    onClick = {
                        val intent = PreferencesActivity.newIntent(context, PreferencesActivity.GENERAL_PREFERENCES)
                        startActivityWithSlideInAnimation(intent)
                    }
                },
                secondaryDrawerItem {
                    nameRes = R.string.about_title_activity
                    iconRes = R.drawable.ic_info_24dp
                    onClick = {
                        val intent = Intent(context, AboutActivity::class.java)
                        startActivityWithSlideInAnimation(intent)
                    }
                },
                secondaryDrawerItem {
                    nameRes = R.string.action_logout
                    iconRes = R.drawable.ic_logout_24dp
                    onClick = { logout(false) }
                }
            )

            if (addSearchButton) {
                binding.mainDrawer.addItemsAtPosition(
                    4,
                    primaryDrawerItem {
                        nameRes = R.string.action_search
                        iconRes = R.drawable.ic_search_24dp
                        onClick = {
                            startActivityWithSlideInAnimation(SearchActivity.getIntent(context))
                        }
                    }
                )
            }

        }

        // Warpdroid: developer tools relied on the removed local timeline cache.
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(binding.mainDrawer.saveInstanceState(outState))
    }

    private fun setupTabs(tabs: List<TabData>) {
        val activeTabLayout = if (preferences.getString(PrefKeys.MAIN_NAV_POSITION, "top") == "bottom") {
            binding.topNav.hide()
            binding.bottomTabLayout
        } else {
            binding.bottomNav.hide()
            binding.tabLayout
        }

        // Save the previous tab so it can be restored later
        val previousTab = tabAdapter.tabs.getOrNull(binding.viewPager.currentItem)

        // Detach any existing mediator before changing tab contents and attaching a new mediator
        tabLayoutMediator?.detach()

        directMessageTab = null

        tabAdapter.tabs = tabs
        tabAdapter.notifyItemRangeChanged(0, tabs.size)

        tabLayoutMediator = TabLayoutMediator(activeTabLayout, binding.viewPager, true) { tab: TabLayout.Tab, position: Int ->
            tab.icon = AppCompatResources.getDrawable(this@MainActivity, tabs[position].icon)
            tab.contentDescription = tabs[position].title(this)
            // Warpdroid has no direct-message tab; nothing to badge here.
        }.also { it.attach() }
        updateDirectMessageBadge(viewModel.showDirectMessagesBadge.value)

        val position = previousTab?.let { tabs.indexOfFirst { it == previousTab } }
            .takeIf { it != -1 } ?: 0
        binding.viewPager.setCurrentItem(position, false)

        val pageMargin = resources.getDimensionPixelSize(R.dimen.tab_page_margin)
        binding.viewPager.setPageTransformer(MarginPageTransformer(pageMargin))

        val enableSwipeForTabs = preferences.getBoolean(PrefKeys.ENABLE_SWIPE_FOR_TABS, true)
        binding.viewPager.isUserInputEnabled = enableSwipeForTabs

        onTabSelectedListener?.let {
            activeTabLayout.removeOnTabSelectedListener(it)
        }

        onTabSelectedListener = object : OnTabSelectedListener {
            override fun onTabSelected(tab: TabLayout.Tab) {
                onBackPressedCallback.isEnabled = tab.position > 0

                binding.mainToolbar.title = tab.contentDescription

                if (tab == directMessageTab) {
                    viewModel.dismissDirectMessagesBadge()
                }
            }

            override fun onTabUnselected(tab: TabLayout.Tab) {}

            override fun onTabReselected(tab: TabLayout.Tab) {
                val fragment = tabAdapter.getFragment(tab.position)
                if (fragment is ReselectableFragment) {
                    fragment.onReselect()
                }
            }
        }.also {
            activeTabLayout.addOnTabSelectedListener(it)
        }

        supportActionBar?.title = tabs[position].title(this@MainActivity)
        binding.mainToolbar.setOnClickListener {
            (
                tabAdapter.getFragment(
                    activeTabLayout.selectedTabPosition
                ) as? ReselectableFragment
                )?.onReselect()
        }
    }

    private fun handleProfileClick(profile: IProfile, current: Boolean): Boolean {
        // open profile when active image was clicked
        if (current) {
            val intent = AccountActivity.newIntent(this, activeAccount.accountId)
            startActivityWithSlideInAnimation(intent)
            return false
        }
        // "Add account" is a no-op in Warpdroid — single stub account only.
        if (profile.identifier == DRAWER_ITEM_ADD_ACCOUNT) {
            return false
        }
        // change Account
        changeAccount(profile.identifier, null)
        return false
    }

    private fun changeAccount(
        @Suppress("UNUSED_PARAMETER") newSelectedId: Long,
        forward: Intent?,
    ) = lifecycleScope.launch {
        val intent = Intent(this@MainActivity, MainActivity::class.java)
        if (forward != null) {
            intent.type = forward.type
            intent.action = forward.action
            intent.putExtras(forward)
        }
        intent.flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
        startActivity(intent)
        finish()
    }

    private fun logout(unauthorized: Boolean) {
        val title = if (unauthorized) {
            R.string.action_remove_account
        } else {
            R.string.action_logout
        }

        val message = if (unauthorized) {
            getString(R.string.action_remove_account_confirm, activeAccount.fullName)
        } else {
            getString(R.string.action_logout_confirm, activeAccount.fullName)
        }

        MaterialAlertDialogBuilder(this)
            .setTitle(title)
            .setMessage(message)
            .setPositiveButton(android.R.string.ok) { _: DialogInterface?, _: Int ->
                binding.appBar.hide()
                binding.viewPager.hide()
                binding.progressBar.show()
                binding.bottomNav.hide()
                binding.composeButton.hide()

                // Logout has no destination in Warpdroid — the app simply restarts
                // into the same stub account.
                lifecycleScope.launch {
                    logoutUsecase.logout(activeAccount)
                    startActivity(Intent(this@MainActivity, MainActivity::class.java))
                    finish()
                }
            }
            .setNegativeButton(android.R.string.cancel) { _, _ ->
                if (unauthorized) {
                    showUnauthorizedWarning(true)
                }
            }
            .show()
    }

    @SuppressLint("CheckResult")
    private fun loadDrawerAvatar(avatarUrl: String, showPlaceholder: Boolean = true) {
        val hideTopToolbar = preferences.getBoolean(PrefKeys.HIDE_TOP_TOOLBAR, false)
        val animateAvatars = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false)

        val activeToolbar = if (hideTopToolbar) {
            val navOnBottom = preferences.getString(PrefKeys.MAIN_NAV_POSITION, "top") == "bottom"
            if (navOnBottom) {
                binding.bottomNav
            } else {
                binding.topNav
            }
        } else {
            binding.mainToolbar
        }

        val navIconSize = resources.getDimensionPixelSize(R.dimen.avatar_toolbar_nav_icon_size)

        if (avatarUrl.isBlank()) {
            // No URL to load — set the default avatar on the toolbar
            // instead of letting Glide log "Load failed for []" or
            // leaving the nav icon stale from a previous profile.
            ContextCompat.getDrawable(this, R.drawable.avatar_default)?.let {
                activeToolbar.navigationIcon = FixedSizeDrawable(it, navIconSize, navIconSize)
            }
            return
        }

        if (animateAvatars) {
            Glide.with(this)
                .asDrawable()
                .load(avatarUrl)
                .transform(
                    RoundedCorners(resources.getDimensionPixelSize(R.dimen.avatar_radius_36dp))
                )
                .apply {
                    if (showPlaceholder) placeholder(R.drawable.avatar_default)
                }
                .into(object : CustomTarget<Drawable>(navIconSize, navIconSize) {

                    override fun onLoadStarted(placeholder: Drawable?) {
                        placeholder?.let {
                            activeToolbar.navigationIcon = FixedSizeDrawable(it, navIconSize, navIconSize)
                        }
                    }

                    override fun onResourceReady(
                        resource: Drawable,
                        transition: Transition<in Drawable>?
                    ) {
                        if (resource is Animatable) resource.start()
                        activeToolbar.navigationIcon = FixedSizeDrawable(resource, navIconSize, navIconSize)
                    }

                    override fun onLoadCleared(placeholder: Drawable?) {
                        placeholder?.let {
                            activeToolbar.navigationIcon = FixedSizeDrawable(it, navIconSize, navIconSize)
                        }
                    }
                })
        } else {
            Glide.with(this)
                .asBitmap()
                .load(avatarUrl)
                .transform(
                    RoundedCorners(resources.getDimensionPixelSize(R.dimen.avatar_radius_36dp))
                )
                .apply {
                    if (showPlaceholder) placeholder(R.drawable.avatar_default)
                }
                .into(object : CustomTarget<Bitmap>(navIconSize, navIconSize) {
                    override fun onLoadStarted(placeholder: Drawable?) {
                        placeholder?.let {
                            activeToolbar.navigationIcon = FixedSizeDrawable(it, navIconSize, navIconSize)
                        }
                    }

                    override fun onResourceReady(
                        resource: Bitmap,
                        transition: Transition<in Bitmap>?
                    ) {
                        activeToolbar.navigationIcon = FixedSizeDrawable(
                            resource.toDrawable(resources),
                            navIconSize,
                            navIconSize
                        )
                    }

                    override fun onLoadCleared(placeholder: Drawable?) {
                        placeholder?.let {
                            activeToolbar.navigationIcon = FixedSizeDrawable(it, navIconSize, navIconSize)
                        }
                    }
                })
        }
    }

    private fun updateProfiles(accounts: List<AccountViewData>) {
        if (accounts.isEmpty()) {
            return
        }
        val activeProfile = accounts.first()

        loadDrawerAvatar(activeProfile.profilePictureUrl)

        loadHeader(activeProfile.profileHeaderUrl, header.accountHeaderBackground)

        val profiles: MutableList<IProfile> =
            accounts.map { acc ->
                ProfileDrawerItem().apply {
                    isSelected = acc == activeProfile
                    nameText = acc.displayName
                    // iconUrl on warpnet:// triggers setImageURI sync IO on main; real avatar is
                    // Glide-loaded into header.currentProfileView below.
                    iconRes = R.drawable.avatar_default
                    isNameShown = true
                    identifier = acc.id
                    descriptionText = "@${acc.username}"
                }
            }.toMutableList()

        // reuse the already existing "add account" item
        for (profile in header.profiles.orEmpty()) {
            if (profile.identifier == DRAWER_ITEM_ADD_ACCOUNT) {
                profiles.add(profile)
                break
            }
        }
        header.clear()
        header.profiles = profiles
        header.setActiveProfile(activeProfile.id)
        // Load the active profile's real avatar async — ProfileDrawerItem only carries the placeholder.
        if (activeProfile.profilePictureUrl.isNotBlank()) {
            header.currentProfileView?.let { profileImageView ->
                val animateAvatars = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false)
                val manager = Glide.with(profileImageView)
                if (animateAvatars) {
                    manager.asDrawable()
                        .load(activeProfile.profilePictureUrl)
                        .placeholder(R.drawable.avatar_default)
                        .into(profileImageView)
                } else {
                    manager.asBitmap()
                        .load(activeProfile.profilePictureUrl)
                        .placeholder(R.drawable.avatar_default)
                        .into(profileImageView)
                }
            }
        }
        binding.mainToolbar.subtitle = if (accountManager.shouldDisplaySelfUsername()) {
            activeProfile.fullName
        } else {
            null
        }
    }

    override fun getActionButton() = binding.composeButton

    @SuppressLint("BatteryLife")
    private fun requestIgnoreBatteryOptimizations() {
        val powerManager = getSystemService(PowerManager::class.java) ?: return
        if (powerManager.isIgnoringBatteryOptimizations(packageName)) {
            return
        }
        if (preferences.getBoolean(PrefKeys.ASKED_IGNORE_BATTERY_OPTIMIZATIONS, false)) {
            return
        }
        preferences.edit { putBoolean(PrefKeys.ASKED_IGNORE_BATTERY_OPTIMIZATIONS, true) }
        try {
            requestIgnoreBatteryOptimizationsLauncher.launch(
                Intent(
                    Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                    Uri.parse("package:$packageName"),
                ),
            )
        } catch (e: ActivityNotFoundException) {
            Log.w(TAG, "No activity to handle battery optimization exemption request", e)
        }
    }

    companion object {
        private const val TAG = "MainActivity" // logging tag
        private const val DRAWER_ITEM_ADD_ACCOUNT: Long = -13
        private const val REDIRECT_URL = "redirectUrl"
        private const val OPEN_DRAFTS = "draft"
        private const val WARPDROID_ACCOUNT_ID = "warpdroidAccountId"
        private const val COMPOSE_OPTIONS = "composeOptions"
        private const val NOTIFICATION_TYPE = "notificationType"
        private const val NOTIFICATION_TAG = "notificationTag"
        private const val NOTIFICATION_ID = "notificationId"

        /**
         * Switches the active account to the provided accountId and then stays on MainActivity
         */
        @JvmStatic
        fun accountSwitchIntent(context: Context, warpdroidAccountId: Long): Intent {
            return Intent(context, MainActivity::class.java).apply {
                putExtra(WARPDROID_ACCOUNT_ID, warpdroidAccountId)
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
            }
        }

        /**
         * Switches the active account to the accountId and takes the user to the correct place according to the notification they clicked
         */
        @JvmStatic
        fun openNotificationIntent(
            context: Context,
            warpdroidAccountId: Long,
            type: Notification.Type
        ): Intent {
            return accountSwitchIntent(context, warpdroidAccountId).apply {
                putExtra(NOTIFICATION_TYPE, type.name)
            }
        }

        /**
         * Switches the active account to the accountId and then opens ComposeActivity with the provided options
         * @param warpdroidAccountId the id of the Warpdroid account to open the screen with. Set to -1 for current account.
         * @param notificationId optional id of the notification that should be cancelled when this intent is opened
         * @param notificationTag optional tag of the notification that should be cancelled when this intent is opened
         */
        @JvmStatic
        fun composeIntent(
            context: Context,
            options: ComposeActivity.ComposeOptions,
            warpdroidAccountId: Long = -1,
            notificationTag: String? = null,
            notificationId: Int = -1
        ): Intent {
            return accountSwitchIntent(context, warpdroidAccountId).apply {
                action = Intent.ACTION_SEND // so it can be opened via shortcuts
                putExtra(COMPOSE_OPTIONS, options)
                putExtra(NOTIFICATION_TAG, notificationTag)
                putExtra(NOTIFICATION_ID, notificationId)
            }
        }

        /**
         * switches the active account to the accountId and then tries to resolve and show the provided url
         */
        @JvmStatic
        fun redirectIntent(context: Context, warpdroidAccountId: Long, url: String): Intent {
            return accountSwitchIntent(context, warpdroidAccountId).apply {
                putExtra(REDIRECT_URL, url)
            }
        }

        /**
         * switches the active account to the provided accountId and then opens drafts
         */
        fun draftIntent(context: Context, warpdroidAccountId: Long): Intent {
            return accountSwitchIntent(context, warpdroidAccountId).apply {
                putExtra(OPEN_DRAFTS, true)
            }
        }
    }
}

private inline fun primaryDrawerItem(block: PrimaryDrawerItem.() -> Unit): PrimaryDrawerItem {
    return PrimaryDrawerItem()
        .apply {
            isSelectable = false
            isIconTinted = true
        }
        .apply(block)
}

private inline fun secondaryDrawerItem(block: SecondaryDrawerItem.() -> Unit): SecondaryDrawerItem {
    return SecondaryDrawerItem()
        .apply {
            isSelectable = false
            isIconTinted = true
        }
        .apply(block)
}

private var AbstractDrawerItem<*, *>.onClick: () -> Unit
    get() = throw UnsupportedOperationException()
    set(value) {
        onDrawerItemClickListener = { _, _, _ ->
            value()
            false
        }
    }
