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
import com.keylesspalace.tusky.components.report.adapter.RuleAdapter
import com.keylesspalace.tusky.databinding.FragmentReportRulesBinding
import com.keylesspalace.tusky.util.Success
import com.keylesspalace.tusky.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.launch

@AndroidEntryPoint
class ReportRulesFragment : Fragment(R.layout.fragment_report_rules) {

    private val viewModel: ReportViewModel by activityViewModels()

    private val binding by viewBinding(FragmentReportRulesBinding::bind)

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.rules.collect { ruleRes ->
                if (ruleRes is Success && ruleRes.data != null) {
                    binding.ruleList.adapter = RuleAdapter(ruleRes.data, viewModel::toggleRule)
                }
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.selectedRules.collect { selectedRules ->
                (binding.ruleList.adapter as RuleAdapter).selectedRules = selectedRules
                binding.buttonContinue.isEnabled = selectedRules.isNotEmpty()
            }
        }

        binding.buttonBack.setOnClickListener {
            viewModel.backFrom(ReportViewModel.Screen.Rules)
        }

        binding.buttonContinue.setOnClickListener {
            viewModel.forwardFrom(ReportViewModel.Screen.Rules)
        }
    }

    companion object {
        fun newInstance() = ReportRulesFragment()
    }
}
