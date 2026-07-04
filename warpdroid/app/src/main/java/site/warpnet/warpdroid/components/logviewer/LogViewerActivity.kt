/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */
package site.warpnet.warpdroid.components.logviewer

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.lifecycleScope
import dagger.hilt.android.AndroidEntryPoint
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import javax.inject.Inject
import kotlinx.coroutines.launch
import site.warpnet.transport.NodeLogSink
import site.warpnet.transport.WarpnetClient
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.ui.WarpdroidTheme

/**
 * Developer-tools screen showing the in-memory [LogBuffer]: node (Go) and
 * app (Kotlin) log lines with level/source filtering, autoscroll, and
 * clear / copy / share actions. Reached from Preferences → Developer tools.
 */
@AndroidEntryPoint
class LogViewerActivity : BaseActivity() {

    @Inject
    lateinit var logBuffer: LogBuffer

    @Inject
    lateinit var warpnetClient: WarpnetClient

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        setContent {
            WarpdroidTheme {
                LogViewerScreen(
                    buffer = logBuffer,
                    onVerboseGoLogs = { verbose ->
                        lifecycleScope.launch {
                            warpnetClient.setNodeLogMinLevel(
                                if (verbose) NodeLogSink.LEVEL_DEBUG else NodeLogSink.LEVEL_INFO
                            )
                        }
                    },
                    onBack = { finish() },
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LogViewerScreen(
    buffer: LogBuffer,
    onVerboseGoLogs: (Boolean) -> Unit,
    onBack: () -> Unit,
) {
    val context = androidx.compose.ui.platform.LocalContext.current
    val scope = rememberCoroutineScope()

    val entries = remember { mutableStateListOf<LogBuffer.Entry>() }
    LaunchedEffect(Unit) {
        entries.clear()
        entries.addAll(buffer.snapshot())
        buffer.updates.collect { entry ->
            entries.add(entry)
            if (entries.size > LogBuffer.CAPACITY) {
                entries.removeAt(0)
            }
        }
    }

    var minLevel by rememberSaveable { mutableStateOf(NodeLogSink.LEVEL_DEBUG) }
    var sourceFilter by rememberSaveable { mutableStateOf<String?>(null) } // null | "GO" | "KOTLIN"
    var autoscroll by rememberSaveable { mutableStateOf(true) }
    var verboseGo by rememberSaveable { mutableStateOf(false) }
    var menuOpen by remember { mutableStateOf(false) }

    val filtered = entries.filter { entry ->
        entry.level >= minLevel &&
            (sourceFilter == null || entry.source.name == sourceFilter)
    }

    val listState = rememberLazyListState()
    LaunchedEffect(filtered.size, autoscroll) {
        if (autoscroll && filtered.isNotEmpty()) {
            listState.scrollToItem(filtered.lastIndex)
        }
    }

    val timeFormat = remember { SimpleDateFormat("HH:mm:ss.SSS", Locale.US) }
    fun formatEntry(entry: LogBuffer.Entry): String {
        val level = "DIWE".getOrElse(entry.level) { '?' }
        val source = if (entry.source == LogBuffer.Source.GO) "go" else "ktl"
        return "${timeFormat.format(Date(entry.timeMs))} $level $source/${entry.component}: ${entry.message}"
    }

    fun exportText(): String = filtered.joinToString("\n") { formatEntry(it) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.title_logs)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(painterResource(R.drawable.ic_arrow_back_24dp), "back")
                    }
                },
                actions = {
                    IconButton(onClick = { autoscroll = !autoscroll }) {
                        Icon(
                            Icons.Filled.KeyboardArrowDown,
                            contentDescription = stringResource(R.string.log_autoscroll),
                            tint = if (autoscroll) {
                                MaterialTheme.colorScheme.primary
                            } else {
                                MaterialTheme.colorScheme.onSurfaceVariant
                            },
                        )
                    }
                    IconButton(onClick = { menuOpen = true }) {
                        Icon(Icons.Filled.MoreVert, contentDescription = null)
                    }
                    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.log_verbose_go)) },
                            leadingIcon = {
                                Checkbox(checked = verboseGo, onCheckedChange = null)
                            },
                            onClick = {
                                verboseGo = !verboseGo
                                onVerboseGoLogs(verboseGo)
                            },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.log_clear)) },
                            onClick = {
                                menuOpen = false
                                buffer.clear()
                                entries.clear()
                            },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.log_copy_all)) },
                            onClick = {
                                menuOpen = false
                                scope.launch {
                                    val clipboard =
                                        context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                    clipboard.setPrimaryClip(ClipData.newPlainText("logs", exportText()))
                                    // Android 13+ shows its own clipboard confirmation overlay.
                                    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
                                        Toast.makeText(
                                            context,
                                            R.string.log_copied,
                                            Toast.LENGTH_SHORT,
                                        ).show()
                                    }
                                }
                            },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.action_share)) },
                            onClick = {
                                menuOpen = false
                                val send = Intent(Intent.ACTION_SEND).apply {
                                    type = "text/plain"
                                    putExtra(Intent.EXTRA_TEXT, exportText())
                                }
                                context.startActivity(Intent.createChooser(send, null))
                            },
                        )
                    }
                },
            )
        },
    ) { contentPadding ->
        Column(Modifier.padding(contentPadding).fillMaxSize()) {
            Row(
                Modifier
                    .horizontalScroll(rememberScrollState())
                    .padding(horizontal = 8.dp),
            ) {
                val levels = listOf(
                    NodeLogSink.LEVEL_DEBUG to "D",
                    NodeLogSink.LEVEL_INFO to "I",
                    NodeLogSink.LEVEL_WARN to "W",
                    NodeLogSink.LEVEL_ERROR to "E",
                )
                levels.forEach { (level, label) ->
                    FilterChip(
                        selected = minLevel == level,
                        onClick = { minLevel = level },
                        label = { Text(label) },
                        modifier = Modifier.padding(horizontal = 2.dp),
                    )
                }
                val sources = listOf(
                    null to stringResource(R.string.log_source_all),
                    "GO" to stringResource(R.string.log_source_go),
                    "KOTLIN" to stringResource(R.string.log_source_kotlin),
                )
                sources.forEach { (source, label) ->
                    FilterChip(
                        selected = sourceFilter == source,
                        onClick = { sourceFilter = source },
                        label = { Text(label) },
                        modifier = Modifier.padding(horizontal = 2.dp),
                    )
                }
            }
            LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
                itemsIndexed(filtered) { _, entry ->
                    Text(
                        text = formatEntry(entry),
                        fontFamily = FontFamily.Monospace,
                        fontSize = 11.sp,
                        lineHeight = 14.sp,
                        color = entry.levelColor(),
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 1.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun LogBuffer.Entry.levelColor(): Color = when (level) {
    NodeLogSink.LEVEL_ERROR -> MaterialTheme.colorScheme.error
    NodeLogSink.LEVEL_WARN -> MaterialTheme.colorScheme.tertiary
    NodeLogSink.LEVEL_DEBUG -> MaterialTheme.colorScheme.onSurfaceVariant
    else -> MaterialTheme.colorScheme.onSurface
}
