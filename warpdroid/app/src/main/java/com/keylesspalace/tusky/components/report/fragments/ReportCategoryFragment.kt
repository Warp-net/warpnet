/* Copyright 2025 Tusky Contributors
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

package com.keylesspalace.tusky.components.report.fragments

import android.os.Bundle
import android.view.View
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.lifecycleScope
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.components.report.ReportViewModel
import com.keylesspalace.tusky.databinding.FragmentReportCategoryBinding
import com.keylesspalace.tusky.util.Error
import com.keylesspalace.tusky.util.Loading
import com.keylesspalace.tusky.util.Success
import com.keylesspalace.tusky.util.viewBinding
import com.keylesspalace.tusky.util.visible
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.launch

@AndroidEntryPoint
class ReportCategoryFragment : Fragment(R.layout.fragment_report_category) {

    private val viewModel: ReportViewModel by activityViewModels()

    private val binding by viewBinding(FragmentReportCategoryBinding::bind)

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.rules.collect { rules ->
                binding.reportCategoryViolation.visible(rules is Success && !rules.data.isNullOrEmpty())
                binding.reportCategoryScrollView.visible(rules is Success)
                binding.reportCategoryLoadingIndicator.visible(rules is Loading)
                binding.reportCategoryStatusView.visible(rules is Error)
                if (rules is Error) {
                    binding.reportCategoryStatusView.setup(rules.cause ?: Throwable("unknown error")) { viewModel.loadInstanceRules() }
                }
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.reportCategory.collect { reportReason ->
                binding.reportCategorySpam.isChecked = reportReason == ReportViewModel.ReportCategory.SPAM
                binding.reportCategoryLegal.isChecked = reportReason == ReportViewModel.ReportCategory.LEGAL
                binding.reportCategoryViolation.isChecked = reportReason == ReportViewModel.ReportCategory.VIOLATION
                binding.reportCategoryOther.isChecked = reportReason == ReportViewModel.ReportCategory.OTHER

                binding.buttonContinue.isEnabled = reportReason != null
            }
        }

        binding.reportCategorySpam.setOnClickListener { viewModel.selectReportCategory(ReportViewModel.ReportCategory.SPAM) }
        binding.reportCategoryLegal.setOnClickListener { viewModel.selectReportCategory(ReportViewModel.ReportCategory.LEGAL) }
        binding.reportCategoryViolation.setOnClickListener { viewModel.selectReportCategory(ReportViewModel.ReportCategory.VIOLATION) }
        binding.reportCategoryOther.setOnClickListener { viewModel.selectReportCategory(ReportViewModel.ReportCategory.OTHER) }

        binding.buttonCancel.setOnClickListener {
            viewModel.backFrom(ReportViewModel.Screen.Category)
        }

        binding.buttonContinue.setOnClickListener {
            viewModel.forwardFrom(ReportViewModel.Screen.Category)
        }
    }

    companion object {
        fun newInstance() = ReportCategoryFragment()
    }
}
