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

package site.warpnet.warpdroid.components.filters

import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsBottomHeight
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyItemScope
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.dimensionResource
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Filter
import site.warpnet.warpdroid.ui.MessageViewMode
import site.warpnet.warpdroid.ui.WarpdroidMessageView
import site.warpnet.warpdroid.ui.WarpdroidPullToRefreshBox
import site.warpnet.warpdroid.ui.WarpdroidTextButton
import site.warpnet.warpdroid.ui.WarpdroidTheme
import site.warpnet.warpdroid.ui.warpdroidColors
import site.warpnet.warpdroid.util.getRelativeTimeSpanString
import site.warpnet.warpdroid.util.withSlideInAnimation
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class FiltersActivity : BaseActivity() {

    private val viewModel: FiltersViewModel by viewModels()

    private val editFilterLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
            // refresh the filters upon returning from EditFilterActivity
            viewModel.reload()
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            WarpdroidTheme {
                FiltersActivityContent()
            }
        }
    }

    @OptIn(ExperimentalMaterial3Api::class)
    @Composable
    private fun FiltersActivityContent() {
        val state by viewModel.state.collectAsStateWithLifecycle()

        var showDeleteConfirmation by remember { mutableStateOf<Filter?>(null) }

        val snackbarHostState = remember { SnackbarHostState() }

        Scaffold(
            contentWindowInsets = WindowInsets(0, 0, 0, 0),
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(R.string.pref_title_timeline_filters)) },
                    navigationIcon = {
                        IconButton(
                            onClick = ::finish
                        ) {
                            Icon(
                                painterResource(R.drawable.ic_arrow_back_24dp),
                                stringResource(R.string.button_back)
                            )
                        }
                    }
                )
            },
            floatingActionButton = {
                if (state.error == null && !(state.loading && state.filters.isEmpty())) {
                    Box(
                        modifier = Modifier.safeDrawingPadding()
                    ) {
                        FloatingActionButton(
                            onClick = ::launchEditFilterActivity,
                            containerColor = colorScheme.primary
                        ) {
                            Icon(
                                painter = painterResource(R.drawable.ic_add_24dp),
                                contentDescription = stringResource(R.string.filter_addition_title)
                            )
                        }
                    }
                }
            },
            snackbarHost = {
                SnackbarHost(snackbarHostState)
            }
        ) { contentPadding ->

            WarpdroidPullToRefreshBox(
                isRefreshing = state.loading,
                onRefresh = viewModel::reload,
                modifier = Modifier
                    .padding(contentPadding)
                    .fillMaxSize(),
            ) {
                if (state.loading && state.filters.isEmpty()) {
                    CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
                } else {
                    if (state.error != null) {
                        WarpdroidMessageView(
                            onRetry = viewModel::reload,
                            error = state.error!!,
                            modifier = Modifier.align(Alignment.Center)
                        )
                    } else if (state.filters.isEmpty()) {
                        WarpdroidMessageView(
                            onRetry = null,
                            message = stringResource(R.string.message_filters_empty),
                            mode = MessageViewMode.EMPTY,
                            modifier = Modifier.align(Alignment.Center)
                        )
                    } else {
                        val filters = state.filters.distinctBy { it.id }
                        LazyColumn {
                            items(
                                count = filters.size,
                                key = { index -> filters[index].id }
                            ) { index ->
                                Filter(
                                    filter = filters[index],
                                    onDelete = {
                                        showDeleteConfirmation = filters[index]
                                    }
                                )
                            }
                            item(key = "bottomSpacer") {
                                Spacer(Modifier.windowInsetsBottomHeight(WindowInsets.systemBars))
                                Spacer(Modifier.height(dimensionResource(R.dimen.recyclerview_bottom_padding_actionbutton)))
                            }
                        }
                    }
                }
            }
        }

        val snackBarMessage = state.deletionError?.let { filterWithError ->
            stringResource(R.string.error_deleting_filter, filterWithError.title)
        }

        LaunchedEffect(state.deletionError) {
            if (snackBarMessage != null) {
                snackbarHostState.showSnackbar(snackBarMessage)
                viewModel.clearDeletionError()
            }
        }

        showDeleteConfirmation?.let { filterToDelete ->
            AlertDialog(
                text = { Text(stringResource(R.string.dialog_delete_filter_text, filterToDelete.title)) },
                onDismissRequest = {
                    showDeleteConfirmation = null
                },
                confirmButton = {
                    WarpdroidTextButton(
                        text = stringResource(R.string.dialog_delete_filter_positive_action),
                        onClick = {
                            viewModel.deleteFilter(filterToDelete)
                            showDeleteConfirmation = null
                        }
                    )
                },
                dismissButton = {
                    WarpdroidTextButton(
                        text = stringResource(android.R.string.cancel),
                        onClick = {
                            showDeleteConfirmation = null
                        }
                    )
                },
                shape = RoundedCornerShape(16.dp),
                containerColor = colorScheme.background,
                tonalElevation = 6.dp
            )
        }
    }

    @Composable
    private fun LazyItemScope.Filter(
        filter: Filter,
        onDelete: () -> Unit
    ) {
        val actions = stringArrayResource(R.array.filter_actions)
        val contexts = stringArrayResource(R.array.filter_contexts)

        val title = if (filter.expiresAt == null) {
            filter.title
        } else {
            stringResource(
                R.string.filter_expiration_format,
                filter.title,
                getRelativeTimeSpanString(this@FiltersActivity, filter.expiresAt.time, System.currentTimeMillis())
            )
        }
        val description = stringResource(
            R.string.filter_description_format,
            actions[filter.action.ordinal - 1],
            filter.context.joinToString("/") { contexts[it.ordinal] }
        )
        Column(modifier = Modifier.animateItem()) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .clickable { launchEditFilterActivity(filter) }
                    .padding(horizontal = 16.dp, vertical = 8.dp)
            ) {
                Column(
                    modifier = Modifier.weight(1f)
                ) {
                    Text(
                        text = title,
                        fontWeight = FontWeight.Medium,
                        color = warpdroidColors.primaryTextColor,
                        fontSize = 16.sp
                    )
                    Spacer(Modifier.height(4.dp))
                    Text(
                        text = description,
                        color = warpdroidColors.tertiaryTextColor,
                        fontSize = 14.sp
                    )
                }
                Spacer(modifier = Modifier.width(8.dp))
                IconButton(
                    onClick = onDelete
                ) {
                    Icon(
                        painter = painterResource(R.drawable.ic_close_24dp),
                        contentDescription = stringResource(R.string.action_delete)
                    )
                }
            }
            HorizontalDivider()
        }
    }

    private fun launchEditFilterActivity(filter: Filter? = null) {
        val intent = Intent(this, EditFilterActivity::class.java).apply {
            if (filter != null) {
                putExtra(EditFilterActivity.FILTER_TO_EDIT, filter)
            }
        }.withSlideInAnimation()
        editFilterLauncher.launch(intent)
    }
}
