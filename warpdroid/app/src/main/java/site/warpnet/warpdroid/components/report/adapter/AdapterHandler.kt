/* Copyright 2019 Joel Pyska
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

package site.warpnet.warpdroid.components.report.adapter

import android.view.View
import site.warpnet.warpdroid.entity.Status
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.viewdata.StatusViewData

interface AdapterHandler : LinkListener {
    fun showMedia(v: View?, status: StatusViewData.Concrete, idx: Int)
    fun setStatusChecked(status: Status, isChecked: Boolean)
    fun isStatusChecked(id: String): Boolean
}
