/* Copyright 2021 Tusky Contributors
 *
 * This file is a part of Tusky.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Tusky is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Tusky; if not,
 * see <http://www.gnu.org/licenses>. */

package com.keylesspalace.tusky.components.timeline

import android.content.SharedPreferences
import android.os.Bundle
import android.view.LayoutInflater
import android.view.Menu
import android.view.MenuInflater
import android.view.MenuItem
import android.view.View
import android.view.ViewGroup
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.scrollBy
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshots.Snapshot
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.rememberNestedScrollInteropConnection
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.MenuProvider
import androidx.fragment.app.Fragment
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.paging.compose.LazyPagingItems
import androidx.paging.compose.collectAsLazyPagingItems
import androidx.paging.compose.itemContentType
import androidx.paging.compose.itemKey
import at.connyduck.calladapter.networkresult.onFailure
import at.connyduck.sparkbutton.compose.SparkButtonState
import com.google.android.material.snackbar.Snackbar
import com.keylesspalace.tusky.BottomSheetActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.appstore.EventHub
import com.keylesspalace.tusky.appstore.StatusComposedEvent
import com.keylesspalace.tusky.components.compose.ComposeActivity
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfoRepository
import com.keylesspalace.tusky.components.preference.PreferencesFragment.ReadingOrder
import com.keylesspalace.tusky.components.timeline.viewmodel.NetworkTimelineViewModel
import com.keylesspalace.tusky.components.timeline.viewmodel.TimelineViewModel
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.entity.Filter
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.interfaces.RefreshableFragment
import com.keylesspalace.tusky.interfaces.ReselectableFragment
import com.keylesspalace.tusky.interfaces.StatusActionListener
import com.keylesspalace.tusky.ui.ErrorSnackbars
import com.keylesspalace.tusky.ui.FilteredStatus
import com.keylesspalace.tusky.ui.LoadMorePlaceholder
import com.keylesspalace.tusky.ui.MessageViewMode
import com.keylesspalace.tusky.ui.TuskyMessageView
import com.keylesspalace.tusky.ui.TuskyPullToRefreshBox
import com.keylesspalace.tusky.ui.TuskyTheme
import com.keylesspalace.tusky.ui.preferences.LocalAccount
import com.keylesspalace.tusky.ui.preferences.LocalPreferences
import com.keylesspalace.tusky.ui.preferences.textStyle
import com.keylesspalace.tusky.ui.statuscomponents.Status
import com.keylesspalace.tusky.ui.statuscomponents.StatusPlaceholder
import com.keylesspalace.tusky.ui.statuscomponents.TimelineStatusInfo
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.addIconAnnotations
import com.keylesspalace.tusky.util.iconInlineContent
import com.keylesspalace.tusky.util.isRefreshing
import com.keylesspalace.tusky.util.reply
import com.keylesspalace.tusky.util.report
import com.keylesspalace.tusky.util.startActivityWithSlideInAnimation
import com.keylesspalace.tusky.util.unsafeLazy
import com.keylesspalace.tusky.util.viewAccount
import com.keylesspalace.tusky.util.viewMedia
import com.keylesspalace.tusky.util.viewTag
import com.keylesspalace.tusky.util.viewThread
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmFavourite
import com.keylesspalace.tusky.view.ConfirmationBottomSheet.Companion.confirmReblog
import com.keylesspalace.tusky.viewdata.AttachmentViewData
import com.keylesspalace.tusky.viewdata.StatusViewData
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.launch

@AndroidEntryPoint
class TimelineFragment :
    Fragment(),
    StatusActionListener,
    ReselectableFragment,
    RefreshableFragment,
    MenuProvider {

    @Inject
    lateinit var eventHub: EventHub

    @Inject
    lateinit var preferences: SharedPreferences

    @Inject
    lateinit var accountManager: AccountManager

    @Inject
    lateinit var instanceInfoRepository: InstanceInfoRepository

    private val viewModel: TimelineViewModel by unsafeLazy {
        // Warpdroid: no local timeline cache — always use the network-backed VM.
        ViewModelProvider(
            viewModelStore,
            defaultViewModelProviderFactory,
            defaultViewModelCreationExtras
        )[NetworkTimelineViewModel::class.java]
    }

    private lateinit var kind: TimelineViewModel.Kind

    private var isPullToRefreshEnabled = true

    private val jumpUp: MutableSharedFlow<Unit> = MutableSharedFlow()
    private val refresh: MutableSharedFlow<Unit> = MutableSharedFlow()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val arguments = requireArguments()
        kind = TimelineViewModel.Kind.valueOf(arguments.getString(KIND_ARG)!!)
        val id: String? = if (kind == TimelineViewModel.Kind.USER ||
            kind == TimelineViewModel.Kind.USER_PINNED ||
            kind == TimelineViewModel.Kind.USER_WITH_REPLIES ||
            kind == TimelineViewModel.Kind.LIST ||
            kind == TimelineViewModel.Kind.QUOTES
        ) {
            arguments.getString(ID_ARG)!!
        } else {
            null
        }

        val tags = if (kind == TimelineViewModel.Kind.TAG) {
            arguments.getStringArrayList(HASHTAGS_ARG)!!
        } else {
            listOf()
        }
        viewModel.init(kind, id, tags)

        isPullToRefreshEnabled = arguments.getBoolean(ARG_ENABLE_SWIPE_TO_REFRESH, true)
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = ComposeView(inflater.context)
        view.setContent {
            TuskyTheme {
                TimelineContent()
            }
        }
        return view
    }

    @Composable
    private fun TimelineContent() {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(tuskyColors.windowBackground)
        ) {
            val oldestFirst = LocalPreferences.current.readingOrder == ReadingOrder.OLDEST_FIRST

            val activeAccount = LocalAccount.current ?: return

            val listState = rememberSaveable(saver = LazyListState.Saver) {
                val firstVisibleItemIndex = if (kind == TimelineViewModel.Kind.HOME && oldestFirst) {
                    activeAccount.firstVisibleHomeTimelineItemIndex
                } else {
                    0
                }
                val firstVisibleItemScrollOffset = if (kind == TimelineViewModel.Kind.HOME && oldestFirst) {
                    activeAccount.firstVisibleHomeTimelineItemOffset
                } else {
                    0
                }
                LazyListState(
                    firstVisibleItemIndex = firstVisibleItemIndex,
                    firstVisibleItemScrollOffset = firstVisibleItemScrollOffset
                )
            }

            val statuses = viewModel.statuses.collectAsLazyPagingItems()

            if (viewModel.kind == TimelineViewModel.Kind.HOME && oldestFirst) {
                LifecycleEventEffect(Lifecycle.Event.ON_STOP) {
                    viewModel.saveHomeTimelinePosition(listState.firstVisibleItemIndex, listState.firstVisibleItemScrollOffset)
                }
            }

            LaunchedEffect(Unit) {
                jumpUp.collect {
                    listState.scrollToItem(0)
                }
            }

            LaunchedEffect(Unit) {
                refresh.collect {
                    statuses.refresh()
                }
            }

            StatusCreatedEffect(statuses)

            if (statuses.itemCount == 0) {
                Box(
                    modifier = Modifier
                        .widthIn(max = 640.dp)
                        .fillMaxSize()
                        .align(Alignment.Center)
                        .background(colorScheme.background)
                ) {
                    val error = (statuses.loadState.source.refresh as? LoadState.Error)?.error ?: (statuses.loadState.mediator?.refresh as? LoadState.Error)?.error
                    if (statuses.loadState.isRefreshing()) {
                        CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                    } else if (error != null) {
                        TuskyMessageView(
                            onRetry = statuses::retry,
                            error = error,
                            modifier = Modifier.align(Alignment.Center)
                        )
                    } else {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            modifier = Modifier.fillMaxWidth()
                        ) {
                            if (kind == TimelineViewModel.Kind.HOME) {
                                Text(
                                    text = addIconAnnotations(stringResource(R.string.help_empty_home)),
                                    style = textStyle(16.sp),
                                    color = tuskyColors.primaryTextColor,
                                    inlineContent = iconInlineContent(tuskyColors.primaryTextColor),
                                    modifier = Modifier
                                        .padding(16.dp)
                                        .background(colorScheme.surface, RoundedCornerShape(8.dp))
                                        .padding(horizontal = 16.dp, vertical = 8.dp)
                                )
                            }
                            TuskyMessageView(
                                modifier = Modifier.weight(1f),
                                onRetry = null,
                                message = stringResource(R.string.message_empty),
                                mode = MessageViewMode.EMPTY,
                            )
                        }
                    }
                }
            } else {
                StatusTopLoadedEffect(listState, statuses)

                var idOfItemBelow: String? by remember { mutableStateOf(null) }

                OptionalPullToRefreshBox(
                    statuses = statuses,
                    modifier = Modifier.fillMaxSize(),
                ) {
                    Box(
                        modifier = Modifier
                            .widthIn(max = 640.dp)
                            .fillMaxWidth()
                            .fillMaxHeight()
                            .align(Alignment.Center)
                            .background(colorScheme.background)
                    )

                    val instanceInfo by instanceInfoRepository.instanceInfoFlow().collectAsStateWithLifecycle(instanceInfoRepository.defaultInstanceInfo)
                    val accounts by accountManager.accountsFlow.collectAsStateWithLifecycle()

                    var itemCount: Int by remember { mutableIntStateOf(statuses.itemCount) }
                    var firstItemId: String? by remember { mutableStateOf(statuses.getOptId(0)) }
                    var lastItemId: String? by remember { mutableStateOf(statuses.getOptId(statuses.itemCount - 1)) }

                    LazyColumn(
                        state = listState,
                        modifier = Modifier
                            .fillMaxSize()
                            .nestedScroll(rememberNestedScrollInteropConnection()),
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        items(
                            count = statuses.itemCount,
                            contentType = statuses.itemContentType { viewData ->
                                if (viewData is StatusViewData.Concrete) {
                                    "concrete"
                                } else {
                                    "loadMore"
                                }
                            },
                            key = statuses.itemKey { it.id }
                        ) { index ->
                            when (val viewData = statuses[index]) {
                                null -> {
                                    StatusPlaceholder(
                                        modifier = Modifier.widthIn(max = 640.dp)
                                    )
                                }

                                is StatusViewData.Concrete -> {
                                    if (viewData.filterActive && viewData.filter?.action == Filter.Action.WARN) {
                                        FilteredStatus(
                                            filterTitle = viewData.filter.title,
                                            onReveal = {
                                                viewModel.changeFilter(false, viewData)
                                            },
                                            modifier = Modifier.widthIn(max = 640.dp)
                                        )
                                    } else {
                                        Status(
                                            statusViewData = viewData,
                                            listener = this@TimelineFragment,
                                            statusInfo = {
                                                TimelineStatusInfo(
                                                    statusViewData = viewData,
                                                    listener = this@TimelineFragment
                                                )
                                            },
                                            translationEnabled = instanceInfo.translationEnabled,
                                            accounts = accounts,
                                            modifier = Modifier.widthIn(max = 640.dp)
                                        )
                                    }
                                }

                                is StatusViewData.LoadMore -> {
                                    LoadMorePlaceholder(
                                        loading = viewData.isLoading,
                                        onLoadMore = {
                                            viewModel.loadMore(viewData.id)
                                            val itemAfter = listState.layoutInfo.visibleItemsInfo.find { it.index == index + 1 }
                                            idOfItemBelow = itemAfter?.key as? String?
                                        },
                                        modifier = Modifier.widthIn(max = 640.dp)
                                    )
                                }
                            }
                        }

                        item(key = "bottomSpacer") {
                            Column {
                                Spacer(
                                    modifier = Modifier.windowInsetsBottomHeight(WindowInsets.systemBars)
                                )
                                Spacer(
                                    modifier = Modifier.height(dimensionResource(R.dimen.recyclerview_bottom_padding_actionbutton))
                                )
                            }
                        }

                        if (oldestFirst) {
                            Snapshot.withoutReadObservation {
                                if (idOfItemBelow != null && statuses.itemCount > itemCount && firstItemId == statuses.getOptId(0) && lastItemId == statuses.getOptId(statuses.itemCount - 1)) {
                                    // items got inserted but not at the top or bottom -> must be a load more insert
                                    // check if the item that was below the "load more" gap is still visible
                                    listState.layoutInfo.visibleItemsInfo.find { itemInfo -> itemInfo.key == idOfItemBelow }?.offset?.let { offsetOfItemBelow ->
                                        // if it is, find its new index
                                        val itemBelow = statuses.itemSnapshotList.find { it?.id == idOfItemBelow }
                                        val indexOfItemBelow = statuses.itemSnapshotList.indexOf(itemBelow)
                                        // make sure it stays at the current position when the new inserted items are rendered
                                        listState.requestScrollToItem(indexOfItemBelow, offsetOfItemBelow * -1)
                                    }

                                    idOfItemBelow = null
                                }
                                itemCount = statuses.itemCount
                                firstItemId = statuses.getOptId(0)
                                lastItemId = statuses.getOptId(statuses.itemCount - 1)
                            }
                        }
                    }
                }
            }
            ErrorSnackbars(
                viewModel = viewModel,
                legacyFallback = true,
                modifier = Modifier
                    .align(Alignment.BottomStart)
                    .windowInsetsPadding(WindowInsets.systemBars)
            )
        }
    }

    @Composable
    private fun OptionalPullToRefreshBox(
        statuses: LazyPagingItems<StatusViewData>,
        modifier: Modifier = Modifier,
        content: @Composable () -> Unit,
    ) {
        if (isPullToRefreshEnabled) {
            var isUserRefresh by remember { mutableStateOf(false) }

            LaunchedEffect(statuses.loadState) {
                if (isUserRefresh &&
                    statuses.loadState.refresh !is LoadState.Loading &&
                    statuses.loadState.source.refresh !is LoadState.Loading &&
                    statuses.loadState.mediator?.refresh !is LoadState.Loading
                ) {
                    isUserRefresh = false
                }
            }

            TuskyPullToRefreshBox(
                isRefreshing = isUserRefresh,
                onRefresh = {
                    isUserRefresh = true
                    statuses.refresh()
                },
                modifier = modifier
            ) {
                content()
            }
        } else {
            Box(
                modifier = modifier,
            ) {
                content()
            }
        }
    }

    /** makes the timeline refresh when a new post was created **/
    @Composable
    private fun StatusCreatedEffect(statuses: LazyPagingItems<StatusViewData>) {
        LaunchedEffect(Unit) {
            eventHub.events
                .filterIsInstance<StatusComposedEvent>()
                .collect { event ->
                    val status = event.status
                    when (kind) {
                        TimelineViewModel.Kind.HOME,
                        TimelineViewModel.Kind.PUBLIC_FEDERATED,
                        TimelineViewModel.Kind.PUBLIC_LOCAL,
                        TimelineViewModel.Kind.PUBLIC_TRENDING_STATUSES -> statuses.refresh()

                        TimelineViewModel.Kind.USER,
                        TimelineViewModel.Kind.USER_WITH_REPLIES -> if (status.account.id == viewModel.id) {
                            statuses.refresh()
                        }

                        TimelineViewModel.Kind.TAG,
                        TimelineViewModel.Kind.FAVOURITES,
                        TimelineViewModel.Kind.LIST,
                        TimelineViewModel.Kind.BOOKMARKS,
                        TimelineViewModel.Kind.USER_PINNED,
                        TimelineViewModel.Kind.QUOTES -> return@collect
                    }
                }
        }
    }

    /** move the timeline down slightly when new posts at the top have been loaded **/
    @Composable
    private fun StatusTopLoadedEffect(listState: LazyListState, statuses: LazyPagingItems<StatusViewData>) {
        val jumpUpDistance = with(LocalDensity.current) { -32.dp.toPx() }

        var previousTopId: String? by remember { mutableStateOf(null) }

        val firstVisibleItemIndex = listState.firstVisibleItemIndex
        val firstVisibleItemScrollOffset = listState.firstVisibleItemScrollOffset

        // when the list is scrolled all the way to the top, save the id of the topmost item
        if (listState.firstVisibleItemIndex == 0 && listState.firstVisibleItemScrollOffset == 0) {
            previousTopId = statuses.peek(0)?.id
        }

        // when new items are inserted and the first visible item is the one that was previously on top,
        // jump up a bit to reveal the newly loaded items
        LaunchedEffect(statuses.itemCount, statuses.peek(0)) {
            if (firstVisibleItemScrollOffset == 0 && firstVisibleItemIndex == 0 && statuses.peek(0)?.id == previousTopId) {
                listState.scrollBy(jumpUpDistance)
            }
        }
    }

    fun LazyPagingItems<StatusViewData>.getOptId(pos: Int): String? {
        return if (pos >= 0 && this.itemCount > pos) {
            this.peek(pos)?.id
        } else {
            null
        }
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        requireActivity().addMenuProvider(this, viewLifecycleOwner, Lifecycle.State.RESUMED)
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.startComposing.collect { composeOptions ->
                val intent = ComposeActivity.newIntent(requireContext(), composeOptions)
                requireContext().startActivityWithSlideInAnimation(intent)
            }
        }
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        if (isPullToRefreshEnabled) {
            menuInflater.inflate(R.menu.fragment_timeline, menu)
        }
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_refresh -> {
                if (isPullToRefreshEnabled) {
                    lifecycleScope.launch {
                        refresh.emit(Unit)
                    }
                    true
                } else {
                    false
                }
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
                viewModel.reblog(viewData.actionableId, reblog, visibility)
                state?.animate()
            }
        } else {
            viewModel.reblog(viewData.actionableId, reblog, visibility ?: Status.Visibility.PUBLIC)
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
                viewModel.favorite(viewData.actionableId, true)
                state?.animate()
            }
        } else {
            viewModel.favorite(viewData.actionableId, false)
        }
    }

    override fun onBookmark(viewData: StatusViewData.Concrete, bookmark: Boolean) {
        viewModel.bookmark(viewData.actionableId, bookmark)
    }

    override fun onExpandedChange(viewData: StatusViewData.Concrete, expanded: Boolean) {
        viewModel.changeExpanded(expanded, viewData)
    }

    override fun onContentHiddenChange(viewData: StatusViewData.Concrete, isShowing: Boolean) {
        viewModel.changeContentShowing(isShowing, viewData)
    }

    override fun onContentCollapsedChange(viewData: StatusViewData.Concrete, isCollapsed: Boolean) {
        val status = viewData.asStatusOrNull() ?: return
        viewModel.changeContentCollapsed(isCollapsed, status)
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
        viewModel.redraftStatus(viewData.status)
    }

    override fun onPin(viewData: StatusViewData.Concrete, pin: Boolean) {
        viewModel.pin(viewData.id, pin)
    }

    override fun onViewMedia(viewData: StatusViewData.Concrete, attachmentIndex: Int) {
        requireContext().viewMedia(
            attachmentIndex,
            AttachmentViewData.list(viewData),
        )
    }

    override fun onViewThread(viewData: StatusViewData.Concrete) {
        requireContext().viewThread(viewData)
    }

    override fun onEdit(viewData: StatusViewData.Concrete) {
        viewModel.editStatus(viewData.actionable)
    }

    override fun onReply(viewData: StatusViewData.Concrete) {
        requireContext().reply(viewData, viewModel.activeAccountFlow.value!!)
    }

    override fun onReport(viewData: StatusViewData.Concrete) {
        requireContext().report(viewData)
    }

    override fun onViewTag(tag: String) {
        if (viewModel.kind == TimelineViewModel.Kind.TAG &&
            viewModel.tags.size == 1 &&
            viewModel.tags.contains(tag)
        ) {
            // If already viewing a tag page, then ignore any request to view that tag again.
            return
        }
        requireContext().viewTag(tag)
    }

    override fun onViewAccount(accountId: String) {
        if ((
                viewModel.kind == TimelineViewModel.Kind.USER ||
                    viewModel.kind == TimelineViewModel.Kind.USER_WITH_REPLIES
                ) &&
            viewModel.id == accountId
        ) {
            /* If already viewing an account page, then any requests to view that account page
             * should be ignored. */
            return
        }
        requireContext().viewAccount(accountId)
    }

    override fun onViewUrl(url: String) {
        (requireActivity() as BottomSheetActivity).viewUrl(url)
    }

    override fun onShowQuote(viewData: StatusViewData.Concrete) {
        viewModel.showQuote(viewData)
    }

    override fun removeQuote(viewData: StatusViewData.Concrete) {
        viewModel.removeQuote(viewData.status)
    }

    override fun onReselect() {
        lifecycleScope.launch {
            jumpUp.emit(Unit)
        }
    }

    override fun refreshContent() {
        lifecycleScope.launch {
            refresh.emit(Unit)
        }
    }

    companion object {
        private const val TAG = "TimelineF" // logging tag
        private const val KIND_ARG = "kind"
        private const val ID_ARG = "id"
        private const val HASHTAGS_ARG = "hashtags"
        private const val ARG_ENABLE_SWIPE_TO_REFRESH = "enableSwipeToRefresh"

        fun newInstance(
            kind: TimelineViewModel.Kind,
            hashtagOrId: String? = null,
            enableSwipeToRefresh: Boolean = true
        ): TimelineFragment {
            val fragment = TimelineFragment()
            val arguments = Bundle(3)
            arguments.putString(KIND_ARG, kind.name)
            arguments.putString(ID_ARG, hashtagOrId)
            arguments.putBoolean(ARG_ENABLE_SWIPE_TO_REFRESH, enableSwipeToRefresh)
            fragment.arguments = arguments
            return fragment
        }

        fun newHashtagInstance(hashtags: List<String>): TimelineFragment {
            val fragment = TimelineFragment()
            val arguments = Bundle(3)
            arguments.putString(KIND_ARG, TimelineViewModel.Kind.TAG.name)
            arguments.putStringArrayList(HASHTAGS_ARG, ArrayList(hashtags))
            arguments.putBoolean(ARG_ENABLE_SWIPE_TO_REFRESH, true)
            fragment.arguments = arguments
            return fragment
        }
    }
}
