/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Originally part of Tusky. Warpdroid only ships the Home tab — the aggregated
 * timeline from the paired desktop node. Local, federated, trending,
 * direct-message, hashtag, list, bookmark and notifications tabs have no
 * Warpnet equivalent and are removed.
 */
package com.keylesspalace.tusky

import android.content.Context
import androidx.annotation.DrawableRes
import androidx.annotation.StringRes
import androidx.fragment.app.Fragment
import com.keylesspalace.tusky.components.timeline.TimelineFragment
import com.keylesspalace.tusky.components.timeline.viewmodel.TimelineViewModel
import java.util.Objects

const val HOME = "Home"

data class TabData(
    val id: String,
    @StringRes val text: Int,
    @DrawableRes val icon: Int,
    val fragment: (List<String>) -> Fragment,
    val arguments: List<String> = emptyList(),
    val title: (Context) -> String = { context -> context.getString(text) }
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as TabData
        if (id != other.id) return false
        return arguments == other.arguments
    }

    override fun hashCode() = Objects.hash(id, arguments)
}

fun List<TabData>.hasTab(id: String): Boolean = this.any { it.id == id }

fun createTabDataFromId(id: String, arguments: List<String> = emptyList()): TabData = when (id) {
    HOME -> TabData(
        id = HOME,
        text = R.string.title_home,
        icon = R.drawable.tab_icon_home,
        fragment = { TimelineFragment.newInstance(TimelineViewModel.Kind.HOME) },
    )
    else -> throw IllegalArgumentException("unknown tab type: $id")
}

fun defaultTabs(): List<TabData> = listOf(
    createTabDataFromId(HOME),
)
