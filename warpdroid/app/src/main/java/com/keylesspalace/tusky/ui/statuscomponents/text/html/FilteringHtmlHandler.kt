/* Copyright 2026 Tusky Contributors
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

package com.keylesspalace.tusky.ui.statuscomponents.text.html

class FilteringHtmlHandler(
    private val delegate: HtmlHandler
) : HtmlHandler {

    // Mastodon includes quotes in post text as fallback for clients that don't support native quotes.
    // Since we support native quotes, we need to filter those out.
    private val classToFilter = "quote-inline"

    private var stackSize: Int = 0
    private var filteredTagStackPosition: Int = Int.MAX_VALUE

    override fun onOpenTag(name: String, attributes: (String) -> String?) {
        stackSize++

        if (filteredTagStackPosition > stackSize) {
            if (attributes("class") == classToFilter) {
                filteredTagStackPosition = stackSize
                return
            } else {
                delegate.onOpenTag(name, attributes)
            }
        }
    }

    override fun onCloseTag(name: String) {
        if (filteredTagStackPosition > stackSize) {
            delegate.onCloseTag(name)
        } else if (filteredTagStackPosition == stackSize) {
            filteredTagStackPosition = Int.MAX_VALUE
        }
        stackSize--
    }

    override fun onText(text: String) {
        if (filteredTagStackPosition > stackSize) {
            delegate.onText(text)
        }
    }
}
