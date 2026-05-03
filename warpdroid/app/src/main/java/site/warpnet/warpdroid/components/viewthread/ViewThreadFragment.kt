/* Copyright 2022 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.viewthread

import android.content.SharedPreferences
import android.os.Bundle
import android.view.LayoutInflater
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.View
import android.view.ViewGroup
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SnackbarResult
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.core.view.MenuProvider
import androidx.fragment.app.Fragment
import androidx.fragment.app.commit
import androidx.fragment.app.viewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import at.connyduck.calladapter.networkresult.onFailure
import at.connyduck.sparkbutton.compose.SparkButtonState
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.BottomSheetActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.compose.ComposeActivity
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfoRepository
import site.warpnet.warpdroid.components.viewthread.edits.ViewEditsFragment
import site.warpnet.warpdroid.db.AccountManager
import site.warpnet.warpdroid.db.DraftsAlert
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.entity.Status
import site.warpnet.warpdroid.interfaces.StatusActionListener
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.ui.FilteredStatus
import site.warpnet.warpdroid.ui.WarpdroidMessageView
import site.warpnet.warpdroid.ui.WarpdroidPullToRefreshBox
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.ui.statuscomponents.DetailedStatus
import site.warpnet.warpdroid.ui.statuscomponents.Status
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.openLink
import site.warpnet.warpdroid.util.reply
import site.warpnet.warpdroid.util.report
import site.warpnet.warpdroid.util.startActivityWithSlideInAnimation
import site.warpnet.warpdroid.util.viewAccount
import site.warpnet.warpdroid.util.viewMedia
import site.warpnet.warpdroid.util.viewTag
import site.warpnet.warpdroid.util.viewThread
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmFavourite
import site.warpnet.warpdroid.view.ConfirmationBottomSheet.Companion.confirmReblog
import site.warpnet.warpdroid.viewdata.AttachmentViewData
import site.warpnet.warpdroid.viewdata.StatusViewData
import dagger.hilt.android.AndroidEntryPoint
import dagger.hilt.android.lifecycle.withCreationCallback
import javax.inject.Inject
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@AndroidEntryPoint
class ViewThreadFragment :
    Fragment(),
    StatusActionListener,
    MenuProvider {

    @Inject
    lateinit var preferences: SharedPreferences

    @Inject
    lateinit var draftsAlert: DraftsAlert

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var instanceInfoRepository: InstanceInfoRepository

    private val viewModel: ViewThreadViewModel by viewModels(
        extrasProducer = {
            defaultViewModelCreationExtras.withCreationCallback<ViewThreadViewModel.Factory> { factory ->
                factory.create(
                    threadId = requireArguments().getString(ID_EXTRA)!!,
                )
            }
        }
    )

    /**
     * State of the "reveal" menu item that shows/hides content that is behind a content
     * warning. Setting this invalidates the menu to redraw the menu item.
     */
    private var revealButtonState = RevealButtonState.NO_BUTTON
        set(value) {
            if (field != value) {
                field = value
                requireActivity().invalidateMenu()
            }
        }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = ComposeView(inflater.context)
        view.setContent {
            WarpdroidTheme {
                ViewThreadContent()
            }
        }
        return view
    }

    @Composable
    private fun ViewThreadContent() {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(warpdroidColors.windowBackground)
        ) {
            val uiState by viewModel.uiState.collectAsStateWithLifecycle()

            revealButtonState = uiState.revealButton

            when (uiState) {
                is ThreadUiState.Loading -> {
                    Box(
                        modifier = Modifier
                            .widthIn(max = 640.dp)
                            .fillMaxSize()
                            .align(Alignment.Center)
                            .background(colorScheme.background)
                    ) {
                        CircularProgressIndicator(
                            modifier = Modifier.align(Alignment.Center)
                        )
                    }
                }

                is ThreadUiState.Error -> {
                    Box(
                        modifier = Modifier
                            .widthIn(max = 640.dp)
                            .fillMaxSize()
                            .align(Alignment.Center)
                            .background(colorScheme.background)
                    ) {
                        WarpdroidMessageView(
                            onRetry = viewModel::retry,
                            error = (uiState as ThreadUiState.Error).throwable,
                            modifier = Modifier.align(Alignment.Center)
                        )
                    }
                }

                is ThreadUiState.Success -> {
                    ViewThreadContentList(uiState as ThreadUiState.Success)
                }
            }

            ErrorSnackbars(
                modifier = Modifier
                    .align(Alignment.BottomStart)
                    .windowInsetsPadding(WindowInsets.systemBars)
            )
        }
    }

    @Composable
    private fun ViewThreadContentList(
        uiState: ThreadUiState.Success
    ) {
        val statuses = uiState.statusViewData

        var refreshing by remember { mutableStateOf(false) }

        if (refreshing && !uiState.isRefreshing && !uiState.isloadingThread) {
            refreshing = false
        }

        WarpdroidPullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = {
                refreshing = true
                viewModel.refresh()
            },
            modifier = Modifier.fillMaxSize(),
        ) {
            Box(
                modifier = Modifier
                    .widthIn(max = 640.dp)
                    .fillMaxSize()
                    .align(Alignment.Center)
                    .background(colorScheme.background)
            )

            val instanceInfo by instanceInfoRepository.instanceInfoFlow().collectAsStateWithLifecycle(instanceInfoRepository.defaultInstanceInfo)
            val accounts by accountManager.accountsFlow.collectAsStateWithLifecycle()

            val rtl = LocalLayoutDirection.current == LayoutDirection.Rtl

            val lineColor = warpdroidColors.backgroundAccent
            val avatarMargin = with(LocalDensity.current) { 14.dp.toPx() }
            val lineThickness = with(LocalDensity.current) { 4.dp.toPx() }
            val avatarSize = with(LocalDensity.current) { 48.dp.toPx() }
            val initialScrollOffset = with(LocalDensity.current) { -100.dp.toPx() }.toInt()

            val state = rememberLazyListState(initialFirstVisibleItemIndex = statuses.indexOfFirst { it.isDetailed }, initialFirstVisibleItemScrollOffset = initialScrollOffset)

            // workaround so LazyColumn correctly keeps the position even in large threads https://issuetracker.google.com/issues/273025639
            SideEffect {
                val oldFirstItem = state.layoutInfo.visibleItemsInfo.firstOrNull()
                if (oldFirstItem != null && statuses.getOrNull(oldFirstItem.index)?.id != oldFirstItem.key) {
                    val newIndex = statuses.indexOfFirst { it.id == oldFirstItem.key }
                    if (newIndex != -1) {
                        state.requestScrollToItem(newIndex, state.firstVisibleItemScrollOffset)
                    }
                }
            }

            LazyColumn(
                state = state,
                modifier = Modifier.fillMaxSize(),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                itemsIndexed(
                    items = statuses,
                    key = { _, viewData -> viewData.id },
                ) { position, viewData ->
                    if (viewData.isDetailed) {
                        DetailedStatus(
                            viewData,
                            listener = this@ViewThreadFragment,
                            translationEnabled = instanceInfo.translationEnabled,
                            accounts = accounts,
                            showEdits = {
                                onShowEdits(viewData)
                            },
                            modifier = Modifier
                                .widthIn(max = 640.dp)
                                .drawBehind {
                                    val itemAbove = statuses.getOrNull(position - 1)
                                    if (itemAbove != null && viewData.status.inReplyToId == itemAbove.id) {
                                        val horizontalOffset = if (rtl) {
                                            size.width - avatarMargin - avatarSize / 2
                                        } else {
                                            avatarMargin + avatarSize / 2
                                        }
                                        drawLine(
                                            color = lineColor,
                                            start = Offset(horizontalOffset, 0f),
                                            end = Offset(horizontalOffset, avatarMargin),
                                            strokeWidth = lineThickness
                                        )
                                    }
                                }
                        )
                    } else if (viewData.filterActive && viewData.filter?.action == Filter.Action.WARN) {
                        FilteredStatus(
                            filterTitle = viewData.filter.title,
                            onReveal = {
                                viewModel.changeFilter(false, viewData)
                            },
                            modifier = Modifier.widthIn(max = 640.dp)
                        )
                    } else {
                        Status(
                            viewData,
                            listener = this@ViewThreadFragment,
                            translationEnabled = instanceInfo.translationEnabled,
                            accounts = accounts,
                            modifier = Modifier
                                .widthIn(max = 640.dp)
                                .drawBehind {
                                    val horizontalOffset = if (rtl) {
                                        size.width - avatarMargin - avatarSize / 2
                                    } else {
                                        avatarMargin + avatarSize / 2
                                    }
                                    val itemAbove = statuses.getOrNull(position - 1)
                                    val itemBelow = statuses.getOrNull(position + 1)
                                    if (itemAbove != null && viewData.status.inReplyToId == itemAbove.id) {
                                        drawLine(
                                            color = lineColor,
                                            start = Offset(horizontalOffset, 0f),
                                            end = Offset(horizontalOffset, avatarMargin),
                                            strokeWidth = lineThickness
                                        )
                                    }
                                    if (itemBelow != null && itemBelow.status.inReplyToId == viewData.id) {
                                        drawLine(
                                            color = lineColor,
                                            start = Offset(horizontalOffset, avatarMargin + avatarSize),
                                            end = Offset(horizontalOffset, size.height),
                                            strokeWidth = lineThickness
                                        )
                                    }
                                }
                        )
                    }
                }
                item(key = "bottomSpacer") {
                    Column {
                        Spacer(
                            modifier = Modifier.windowInsetsBottomHeight(WindowInsets.systemBars)
                        )
                        Spacer(
                            modifier = Modifier.height(dimensionResource(R.dimen.recyclerview_bottom_padding_no_actionbutton))
                        )
                    }
                }
            }
            if (uiState.isloadingThread) {
                ThreadLoadingBar(
                    modifier = Modifier
                        .align(Alignment.TopCenter)
                        .fillMaxWidth()
                )
            }
        }
    }

    @Composable
    private fun ThreadLoadingBar(modifier: Modifier = Modifier) {
        var isShown by remember { mutableStateOf(false) }

        LaunchedEffect(Unit) {
            delay(500)
            isShown = true
        }

        if (isShown) {
            LinearProgressIndicator(modifier = modifier)
        }
    }

    @Composable
    private fun ErrorSnackbars(modifier: Modifier = Modifier) {
        val snackbarHostState = remember { SnackbarHostState() }

        SnackbarHost(
            hostState = snackbarHostState,
            modifier = modifier
        )

        val error by viewModel.snackbarErrors.collectAsStateWithLifecycle(initialValue = null)

        val snackbarMessage = error?.message()
        val snackbarActionLabel = error?.retryAction?.let {
            stringResource(R.string.action_retry)
        }

        LaunchedEffect(error) {
            if (snackbarMessage != null) {
                val snackbarResult = snackbarHostState
                    .showSnackbar(
                        message = snackbarMessage,
                        actionLabel = snackbarActionLabel,
                        withDismissAction = true,
                        duration = SnackbarDuration.Long
                    )
                if (error?.retryAction != null && snackbarResult == SnackbarResult.ActionPerformed) {
                    error?.retryAction?.invoke()
                }
                viewModel.snackbarErrorShown()
            }
        }
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        requireActivity().addMenuProvider(this, viewLifecycleOwner, Lifecycle.State.RESUMED)

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.finish.collect {
                activity?.finish()
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.startComposing.collect { composeOptions ->
                val intent = ComposeActivity.newIntent(requireContext(), composeOptions)
                requireContext().startActivityWithSlideInAnimation(intent)
            }
        }
    }

    override fun onResume() {
        super.onResume()
        requireActivity().title = getString(R.string.title_view_thread)
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.fragment_view_thread, menu)
        val actionReveal = menu.findItem(R.id.action_reveal)
        actionReveal.isVisible = revealButtonState != RevealButtonState.NO_BUTTON
        actionReveal.setIcon(
            when (revealButtonState) {
                RevealButtonState.REVEAL -> R.drawable.ic_visibility_24dp
                else -> R.drawable.ic_visibility_off_24dp
            }
        )
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_reveal -> {
                viewModel.toggleRevealButton()
                true
            }

            R.id.action_open_in_web -> {
                context?.openLink(requireArguments().getString(URL_EXTRA)!!)
                true
            }

            R.id.action_refresh -> {
                viewModel.refresh()
                true
            }

            else -> false
        }
    }

    override fun onReblog(
        viewData: StatusViewData.Concrete,
        reblog: Boolean,
        visibility: Status.Visibility?,
        state: SparkButtonState?
    ) {
        if (reblog && visibility == null) {
            confirmReblog(preferences) { visibility ->
                viewModel.reblog(viewData.id, true, visibility)
                state?.animate()
            }
        } else {
            viewModel.reblog(viewData.id, reblog, visibility ?: Status.Visibility.PUBLIC)
            if (reblog) {
                state?.animate()
            }
        }
    }

    override fun onFavourite(
        viewData: StatusViewData.Concrete,
        favourite: Boolean,
        state: SparkButtonState?
    ) {
        if (favourite) {
            confirmFavourite(preferences) {
                viewModel.favorite(viewData.id, true)
                state?.animate()
            }
        } else {
            viewModel.favorite(viewData.id, false)
        }
    }

    override fun onBookmark(viewData: StatusViewData.Concrete, bookmark: Boolean) {
        viewModel.bookmark(viewData.id, bookmark)
    }

    override fun onExpandedChange(viewData: StatusViewData.Concrete, expanded: Boolean) {
        viewModel.changeExpanded(expanded, viewData)
    }

    override fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean) {
        viewModel.changeContentShowing(isShowing, viewData)
    }

    override fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean) {
        viewModel.changeContentCollapsed(isCollapsed, viewData)
    }

    override fun onVoteInPoll(viewData: StatusViewData.Concrete, pollId: String, choices: List<Int>) {
        viewModel.voteInPoll(viewData.actionableId, pollId, choices)
    }

    override fun onShowPollResults(viewData: StatusViewData.Concrete) {
        viewModel.showPollResults(viewData)
    }

    override fun changeFilter(viewData: StatusViewData.Concrete, filtered: Boolean) {
        viewModel.changeFilter(filtered, viewData)
    }

    override fun onTranslate(viewData: StatusViewData.Concrete) {
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.translate(viewData)
                .onFailure {
                    Snackbar.make(
                        requireView(),
                        getString(R.string.ui_error_translate, it.message),
                        Snackbar.LENGTH_LONG
                    ).show()
                }
        }
    }

    override fun onUntranslate(viewData: StatusViewData.Concrete) {
        viewModel.untranslate(viewData)
    }

    override fun onBlock(accountId: String) {
        viewModel.block(accountId)
    }

    override fun onMute(accountId: String, hideNotifications: Boolean, duration: Int?) {
        viewModel.mute(accountId, hideNotifications, duration)
    }

    override fun onMuteConversation(viewData: StatusViewData.Concrete, mute: Boolean) {
        viewModel.muteConversation(viewData.id, mute)
    }

    override fun onDelete(viewData: StatusViewData.Concrete) {
        viewModel.delete(viewData.id)
    }

    override fun onRedraft(viewData: StatusViewData.Concrete) {
        viewModel.redraftStatus(viewData.actionable)
    }

    override fun onPin(viewData: StatusViewData.Concrete, pin: Boolean) {
        viewModel.pin(viewData.id, pin)
    }

    override fun onViewMedia(viewData: StatusViewData.Concrete, attachmentIndex: Int) {
        requireContext().viewMedia(
            attachmentIndex,
            AttachmentViewData.list(viewData, preferences.getBoolean(PrefKeys.ALWAYS_SHOW_SENSITIVE_MEDIA, false))
        )
    }

    override fun onViewThread(viewData: StatusViewData.Concrete) {
        if (viewModel.threadId == viewData.id) {
            // If already viewing this thread, don't reopen it.
            return
        }
        requireContext().viewThread(viewData)
    }

    override fun onEdit(viewData: StatusViewData.Concrete) {
        viewModel.editStatus(viewData.actionable)
    }

    override fun onReply(viewData: StatusViewData.Concrete) {
        requireContext().reply(viewData, accountManager.activeAccount!!)
    }

    override fun onReport(viewData: StatusViewData.Concrete) {
        requireContext().report(viewData)
    }

    override fun onViewUrl(url: String) {
        val status: StatusViewData.Concrete? = (viewModel.uiState.value as? ThreadUiState.Success)?.detailedStatus
        if (status != null && status.status.url == url) {
            // already viewing the status with this url
            // probably just a preview federated and the user is clicking again to view more -> open the browser
            // this can happen with some friendica statuses
            requireContext().openLink(url)
            return
        }
        (requireActivity() as BottomSheetActivity).viewUrl(url)
    }

    override fun onViewTag(tag: String) {
        requireContext().viewTag(tag)
    }

    override fun onViewAccount(accountId: String) {
        requireContext().viewAccount(accountId)
    }

    override fun onShowQuote(viewData: StatusViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: StatusViewData.Concrete) {
        viewModel.removeQuote(viewData.status)
    }

    private fun onShowEdits(viewData: StatusViewData.Concrete) {
        val viewEditsFragment = ViewEditsFragment.newInstance(viewData.actionableId)

        parentFragmentManager.commit {
            setCustomAnimations(
                R.anim.activity_open_enter,
                R.anim.activity_open_exit,
                R.anim.activity_close_enter,
                R.anim.activity_close_exit
            )
            replace(R.id.fragment_container, viewEditsFragment, "ViewEditsFragment_$id")
            addToBackStack(null)
        }
    }

    companion object {
        private const val ID_EXTRA = "id"
        private const val URL_EXTRA = "url"

        fun newInstance(id: String, url: String): ViewThreadFragment {
            val arguments = Bundle(2)
            val fragment = ViewThreadFragment()
            arguments.putString(ID_EXTRA, id)
            arguments.putString(URL_EXTRA, url)
            fragment.arguments = arguments
            return fragment
        }
    }
}
