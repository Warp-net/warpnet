/* Copyright 2018 Conny Duck
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
import android.util.AttributeSet
import android.widget.RadioGroup
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Tweet

class ComposeOptionsView @JvmOverloads constructor(context: Context, attrs: AttributeSet? = null) : RadioGroup(
    context,
    attrs
) {

    var listener: ComposeOptionsListener? = null

    init {
        inflate(context, R.layout.view_compose_options, this)

        setOnCheckedChangeListener { _, checkedId ->
            val visibility = when (checkedId) {
                R.id.publicRadioButton ->
                    Tweet.Visibility.PUBLIC
                R.id.unlistedRadioButton ->
                    Tweet.Visibility.UNLISTED
                R.id.privateRadioButton ->
                    Tweet.Visibility.PRIVATE
                R.id.directRadioButton ->
                    Tweet.Visibility.DIRECT
                else ->
                    Tweet.Visibility.PUBLIC
            }
            listener?.onVisibilityChanged(visibility)
        }
    }

    fun setStatusVisibility(visibility: Tweet.Visibility) {
        val selectedButton = when (visibility) {
            Tweet.Visibility.PUBLIC ->
                R.id.publicRadioButton
            Tweet.Visibility.UNLISTED ->
                R.id.unlistedRadioButton
            Tweet.Visibility.PRIVATE ->
                R.id.privateRadioButton
            Tweet.Visibility.DIRECT ->
                R.id.directRadioButton
            else ->
                R.id.directRadioButton
        }

        check(selectedButton)
    }
}

interface ComposeOptionsListener {
    fun onVisibilityChanged(visibility: Tweet.Visibility)
}
