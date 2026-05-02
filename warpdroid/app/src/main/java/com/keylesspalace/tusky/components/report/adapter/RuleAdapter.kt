/* Copyright 2025 Tusky contributors
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

import android.view.LayoutInflater
import android.view.ViewGroup
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.R as materialR
import com.google.android.material.card.MaterialCardView
import com.google.android.material.color.MaterialColors
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.databinding.ItemReportRuleBinding
import com.keylesspalace.tusky.entity.Instance
import com.keylesspalace.tusky.util.BindingHolder

class RuleAdapter(
    private val rules: List<Instance.Rule>,
    private val onRuleToggled: (ruleId: String) -> Unit
) : RecyclerView.Adapter<BindingHolder<ItemReportRuleBinding>>() {

    var selectedRules: Set<String> = emptySet()

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): BindingHolder<ItemReportRuleBinding> {
        val binding = ItemReportRuleBinding.inflate(LayoutInflater.from(parent.context), parent, false)
        return BindingHolder(binding)
    }

    override fun onBindViewHolder(holder: BindingHolder<ItemReportRuleBinding>, position: Int) {
        val rule = rules[position]
        val isSelected = selectedRules.contains(rule.id)
        holder.binding.ruleCheckBox.text = rule.text

        holder.binding.ruleCheckBox.isChecked = isSelected

        setUpCard(holder.binding.ruleCard, isSelected)

        holder.binding.ruleCheckBox.setOnCheckedChangeListener { view, isChecked ->
            setUpCard(holder.binding.ruleCard, isChecked)
            onRuleToggled(rule.id)
        }
    }

    private fun setUpCard(card: MaterialCardView, isChecked: Boolean) {
        if (isChecked) {
            card.setCardBackgroundColor(MaterialColors.getColor(card, materialR.attr.colorSurface))
            card.strokeColor = MaterialColors.getColor(card, materialR.attr.colorSurface)
        } else {
            card.setCardBackgroundColor(MaterialColors.getColor(card, android.R.attr.colorBackground))
            card.strokeColor = MaterialColors.getColor(card, R.attr.colorBackgroundAccent)
        }
    }

    override fun getItemCount(): Int = rules.size
}
