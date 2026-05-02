/* Copyright 2019 Joel Pyska
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

package com.keylesspalace.tusky.components.report.adapter

import androidx.fragment.app.Fragment
import androidx.fragment.app.FragmentActivity
import androidx.recyclerview.widget.RecyclerView
import androidx.viewpager2.adapter.FragmentStateAdapter
import com.keylesspalace.tusky.components.report.ReportViewModel
import com.keylesspalace.tusky.components.report.fragments.ReportCategoryFragment
import com.keylesspalace.tusky.components.report.fragments.ReportDoneFragment
import com.keylesspalace.tusky.components.report.fragments.ReportNoteFragment
import com.keylesspalace.tusky.components.report.fragments.ReportRulesFragment
import com.keylesspalace.tusky.components.report.fragments.ReportStatusesFragment

class ReportPagerAdapter(activity: FragmentActivity) : FragmentStateAdapter(activity) {

    var showRules: Boolean = false
        set(value) {
            if (field != value) {
                field = value
                if (value) {
                    notifyItemInserted(1)
                } else {
                    notifyItemRemoved(1)
                }
            }
        }

    override fun createFragment(position: Int): Fragment {
        return when (position) {
            0 -> ReportCategoryFragment.newInstance()
            1 -> if (showRules) ReportRulesFragment.newInstance() else ReportStatusesFragment.newInstance()
            2 -> if (showRules) ReportStatusesFragment.newInstance() else ReportNoteFragment.newInstance()
            3 -> if (showRules) ReportNoteFragment.newInstance() else ReportDoneFragment.newInstance()
            4 -> if (showRules) ReportDoneFragment.newInstance() else throw IllegalArgumentException("Unknown page index: $position")
            else -> throw IllegalArgumentException("Unknown page index: $position")
        }
    }

    override fun getItemCount() = if (showRules) 5 else 4

    override fun getItemId(position: Int): Long {
        return when (position) {
            0 -> ReportViewModel.Screen.Category.ordinal
            1 -> if (showRules) {
                ReportViewModel.Screen.Rules.ordinal
            } else {
                ReportViewModel.Screen.Statuses.ordinal
            }
            2 -> if (showRules) {
                ReportViewModel.Screen.Statuses.ordinal
            } else {
                ReportViewModel.Screen.Note.ordinal
            }
            3 -> if (showRules) {
                ReportViewModel.Screen.Note.ordinal
            } else {
                ReportViewModel.Screen.Done.ordinal
            }
            4 -> if (showRules) {
                ReportViewModel.Screen.Done.ordinal
            } else {
                RecyclerView.NO_ID
            }
            else -> RecyclerView.NO_ID
        }.toLong()
    }

    override fun containsItem(itemId: Long): Boolean {
        return if (itemId == ReportViewModel.Screen.Rules.ordinal.toLong()) {
            showRules
        } else {
            true
        }
    }
}
