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
import androidx.appcompat.content.res.AppCompatResources
import com.google.android.material.button.MaterialButton
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.entity.Tweet

class TootButton
@JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = 0
) : MaterialButton(context, attrs, defStyleAttr) {

    private val smallStyle: Boolean = context.resources.getBoolean(R.bool.show_small_toot_button)

    init {
        if (smallStyle) {
            setIconResource(R.drawable.ic_send_24dp)
            iconPadding = 0
        } else {
            setText(R.string.action_send)
        }
        val padding = resources.getDimensionPixelSize(R.dimen.toot_button_horizontal_padding)
        setPadding(padding, 0, padding, 0)
    }

    fun setStatusVisibility(visibility: Tweet.Visibility) {
        if (!smallStyle) {
            icon = when (visibility) {
                Tweet.Visibility.PUBLIC -> {
                    setText(R.string.action_send_public)
                    null
                }
                Tweet.Visibility.UNLISTED -> {
                    setText(R.string.action_send)
                    null
                }
                Tweet.Visibility.PRIVATE,
                Tweet.Visibility.DIRECT -> {
                    setText(R.string.action_send)
                    AppCompatResources.getDrawable(context, R.drawable.ic_lock_24dp)
                }
                else -> {
                    null
                }
            }
        }
    }
}
