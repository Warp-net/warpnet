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

package site.warpnet.warpdroid.components.report.fragments

import android.os.Bundle
import android.view.View
import androidx.fragment.app.Fragment
import androidx.fragment.app.activityViewModels
import androidx.lifecycle.lifecycleScope
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.components.report.ReportViewModel
import site.warpnet.warpdroid.components.report.adapter.RuleAdapter
import site.warpnet.warpdroid.databinding.FragmentReportRulesBinding
import site.warpnet.warpdroid.util.Success
import site.warpnet.warpdroid.util.viewBinding
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
