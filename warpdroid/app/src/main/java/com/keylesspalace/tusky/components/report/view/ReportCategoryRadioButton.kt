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

package com.keylesspalace.tusky.components.report.view

import android.content.Context
import android.util.AttributeSet
import android.view.LayoutInflater
import com.google.android.material.R as materialR
import com.google.android.material.card.MaterialCardView
import com.google.android.material.color.MaterialColors
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.databinding.ViewReportReasonRadioButtonBinding

class ReportCategoryRadioButton @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = materialR.attr.materialCardViewOutlinedStyle
) : MaterialCardView(context, attrs, defStyleAttr) {

    val binding = ViewReportReasonRadioButtonBinding.inflate(LayoutInflater.from(context), this)

    init {
        val styleAttrs = context.theme.obtainStyledAttributes(
            attrs,
            R.styleable.ReportCategoryRadioButton,
            0,
            0
        )

        binding.reportCategoryTitle.text = styleAttrs.getString(R.styleable.ReportCategoryRadioButton_categoryTitle)
        binding.reportCategoryDescription.text = styleAttrs.getString(R.styleable.ReportCategoryRadioButton_categoryDescription)

        styleAttrs.recycle()

        binding.reportCategoryRadioButton.isClickable = false
    }

    override fun setChecked(checked: Boolean) {
        binding.reportCategoryRadioButton.isChecked = checked

        if (checked) {
            setCardBackgroundColor(MaterialColors.getColor(this, materialR.attr.colorSurface))
            strokeColor = MaterialColors.getColor(this, materialR.attr.colorSurface)
        } else {
            setCardBackgroundColor(MaterialColors.getColor(this, android.R.attr.colorBackground))
            strokeColor = MaterialColors.getColor(this, R.attr.colorBackgroundAccent)
        }
    }
}
