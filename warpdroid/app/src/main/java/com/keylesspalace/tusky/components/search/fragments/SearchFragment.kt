package com.keylesspalace.tusky.components.search.fragments

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
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.rememberNestedScrollInteropConnection
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.view.MenuProvider
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.paging.PagingData
import androidx.paging.compose.LazyPagingItems
import androidx.paging.compose.collectAsLazyPagingItems
import com.google.android.material.snackbar.Snackbar
import com.keylesspalace.tusky.BottomSheetActivity
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.StatusListActivity
import com.keylesspalace.tusky.components.account.AccountActivity
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfo
import com.keylesspalace.tusky.components.instanceinfo.InstanceInfoRepository
import com.keylesspalace.tusky.components.search.SearchViewModel
import com.keylesspalace.tusky.db.AccountManager
import com.keylesspalace.tusky.db.entity.AccountEntity
import com.keylesspalace.tusky.interfaces.LinkListener
import com.keylesspalace.tusky.ui.MessageViewMode
import com.keylesspalace.tusky.ui.TuskyMessageView
import com.keylesspalace.tusky.ui.TuskyPullToRefreshBox
import com.keylesspalace.tusky.ui.TuskyTheme
import com.keylesspalace.tusky.util.isRefreshing
import com.keylesspalace.tusky.util.startActivityWithSlideInAnimation
import javax.inject.Inject
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.launch

abstract class SearchFragment<T : Any> :
    Fragment(),
    LinkListener,
    MenuProvider {

    @Inject
    lateinit var instanceInfoRepository: InstanceInfoRepository

    @Inject
    lateinit var accountManager: AccountManager

    protected val viewModel: SearchViewModel by activityViewModels()

    abstract val data: Flow<PagingData<T>>

    private val refresh: MutableSharedFlow<Unit> = MutableSharedFlow()

    abstract fun LazyListScope.searchResult(
        result: LazyPagingItems<T>,
        instanceInfo: InstanceInfo,
        accounts: List<AccountEntity>
    )

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View {
        val view = ComposeView(inflater.context)
        view.setContent {
            TuskyTheme {
                SearchContent()
            }
        }
        return view
    }

    @Composable
    private fun SearchContent() {
        Box {
            val currentQuery by viewModel.currentQuery.collectAsStateWithLifecycle()
            val results = data.collectAsLazyPagingItems()

            LaunchedEffect(Unit) {
                refresh.collect {
                    results.refresh()
                }
            }

            if (results.itemCount == 0) {
                Box(
                    modifier = Modifier
                        .fillMaxHeight()
                        .widthIn(min = 640.dp)
                        .align(Alignment.Center)
                        .background(colorScheme.background)
                ) {
                    val error = (results.loadState.source.refresh as? LoadState.Error)?.error ?: (results.loadState.mediator?.refresh as? LoadState.Error)?.error
                    if (results.loadState.isRefreshing()) {
                        CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                    } else if (error == null && currentQuery.isNotEmpty()) {
                        TuskyMessageView(
                            modifier = Modifier.align(Alignment.Center),
                            onRetry = null,
                            message = stringResource(R.string.search_no_results),
                            mode = MessageViewMode.EMPTY,
                        )
                    }
                }
            } else {
                val instanceInfo by instanceInfoRepository.instanceInfoFlow().collectAsStateWithLifecycle(instanceInfoRepository.defaultInstanceInfo)
                val accounts by accountManager.accountsFlow.collectAsStateWithLifecycle()

                var refreshing by remember { mutableStateOf(false) }

                if (refreshing &&
                    results.loadState.refresh !is LoadState.Loading &&
                    results.loadState.source.refresh !is LoadState.Loading &&
                    results.loadState.mediator?.refresh !is LoadState.Loading
                ) {
                    refreshing = false
                }

                TuskyPullToRefreshBox(
                    isRefreshing = refreshing,
                    onRefresh = {
                        refreshing = true
                        results.refresh()
                    },
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

                    LazyColumn(
                        state = rememberLazyListState(),
                        modifier = Modifier
                            .fillMaxSize()
                            .nestedScroll(rememberNestedScrollInteropConnection()),
                        horizontalAlignment = Alignment.CenterHorizontally
                    ) {
                        searchResult(
                            result = results,
                            instanceInfo = instanceInfo,
                            accounts = accounts
                        )

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
                }
            }

            val snackbarHostState = remember { SnackbarHostState() }

            SnackbarHost(
                hostState = snackbarHostState,
                modifier = Modifier
                    .align(Alignment.BottomStart)
                    .windowInsetsPadding(WindowInsets.safeDrawing)
            )

            val snackBarMessage = stringResource(R.string.failed_search)

            val snackBarAction = stringResource(R.string.action_retry)

            LaunchedEffect(results.loadState.refresh) {
                if (results.loadState.refresh is LoadState.Error) {
                    // TODO once ConversationActivity is also migrated to Compose
                    // The Compose Snackbar is not positioned correctly when inside a CoordinatorLayout with collapsing toolbar
                    // Using old Snackbars for now.
                    /*val snackbarResult = snackbarHostState.showSnackbar(
                        message = snackBarMessage,
                        actionLabel = snackBarAction,
                        duration = SnackbarDuration.Indefinite
                    )
                    if (snackbarResult == SnackbarResult.ActionPerformed) {
                        results.retry()
                    }*/
                    view?.let {
                        Snackbar.make(it, snackBarMessage, Snackbar.LENGTH_INDEFINITE)
                            .setAction(snackBarAction) {
                                results.retry()
                            }
                            .show()
                    }
                }
            }
        }
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        requireActivity().addMenuProvider(this, viewLifecycleOwner, Lifecycle.State.RESUMED)
    }

    override fun onCreateMenu(menu: Menu, menuInflater: MenuInflater) {
        menuInflater.inflate(R.menu.fragment_search, menu)
    }

    override fun onMenuItemSelected(menuItem: MenuItem): Boolean {
        return when (menuItem.itemId) {
            R.id.action_refresh -> {
                lifecycleScope.launch {
                    refresh.emit(Unit)
                }
                true
            }
            else -> false
        }
    }

    override fun onViewAccount(accountId: String) {
        bottomSheetActivity?.startActivityWithSlideInAnimation(
            AccountActivity.newIntent(requireContext(), accountId)
        )
    }

    override fun onViewTag(tag: String) {
        bottomSheetActivity?.startActivityWithSlideInAnimation(
            StatusListActivity.newHashtagIntent(requireContext(), tag)
        )
    }

    override fun onViewUrl(url: String) {
        bottomSheetActivity?.viewUrl(url)
    }

    protected val bottomSheetActivity
        get() = (activity as? BottomSheetActivity)
}
