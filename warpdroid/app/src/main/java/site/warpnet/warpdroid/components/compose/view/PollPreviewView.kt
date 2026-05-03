/* Copyright 2019 Warpdroid Contributors
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

package site.warpnet.warpdroid.components.compose.view

import android.content.Context
import android.content.res.ColorStateList
import android.util.AttributeSet
import android.view.LayoutInflater
import com.google.android.material.R as materialR
import com.google.android.material.card.MaterialCardView
import com.google.android.material.color.MaterialColors
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.adapter.PreviewPollOptionsAdapter
import site.warpnet.warpdroid.databinding.ViewPollPreviewBinding
import site.warpnet.warpdroid.entity.NewPoll

class PollPreviewView @JvmOverloads constructor(
    context: Context?,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = materialR.attr.materialCardViewOutlinedStyle
) :
    MaterialCardView(context, attrs, defStyleAttr) {

    private val adapter = PreviewPollOptionsAdapter()

    private val binding = ViewPollPreviewBinding.inflate(LayoutInflater.from(context), this)

    init {
        setStrokeColor(ColorStateList.valueOf(MaterialColors.getColor(this, materialR.attr.colorOutline)))
        strokeWidth
        elevation = 0f
        binding.pollPreviewOptions.adapter = adapter
    }

    fun setPoll(poll: NewPoll) {
        adapter.update(poll.options, poll.multiple)

        val pollDurationId = resources.getIntArray(R.array.poll_duration_values).indexOfLast {
            it <= poll.expiresIn
        }
        binding.pollDurationPreview.text = resources.getStringArray(R.array.poll_duration_names)[pollDurationId]
    }

    override fun setOnClickListener(l: OnClickListener?) {
        super.setOnClickListener(l)
        adapter.setOnClickListener(l)
    }
}
