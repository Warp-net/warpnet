/* Copyright 2024 Warpdroid contributors
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
package site.warpnet.warpdroid.view

import android.content.Context
import android.util.AttributeSet
import androidx.appcompat.R as appcompatR
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout
import com.google.android.material.color.MaterialColors

/**
 * SwipeRefreshLayout does not allow theming of the color scheme,
 * so we use this class to still have a single point to change its colors.
 */
class WarpdroidSwipeRefreshLayout @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : SwipeRefreshLayout(context, attrs) {

    init {
        setColorSchemeColors(
            MaterialColors.getColor(this, appcompatR.attr.colorPrimary)
        )
    }
}
