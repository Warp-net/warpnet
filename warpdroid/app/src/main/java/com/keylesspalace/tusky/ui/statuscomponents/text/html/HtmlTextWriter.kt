/*
 * Copyright (C) 2023 Christophe Beyls, adapted for Tusky by Conny Duck
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package com.keylesspalace.tusky.ui.statuscomponents.text.html

internal class HtmlTextWriter(
    private val output: Appendable,
    private val callbacks: Callbacks? = null
) {
    interface Callbacks {
        /**
         * Called when new lines are about to be written before content at a block boundary.
         * Allows to override the number of written new lines.
         */
        fun onWriteNewLines(newLineCount: Int): Int

        /**
         * Called when non-empty content is about to be written.
         */
        fun onWriteContentStart()
    }

    private var isBlockStart = true

    // A negative value indicates new lines should be skipped for the next paragraph
    private var pendingNewLineCount = -1

    fun markBlockBoundary(newLineCount: Int) {
        require(newLineCount > 0) { "newLineCount must be positive" }
        pendingNewLineCount.let {
            if (it >= 0) {
                pendingNewLineCount = maxOf(it, newLineCount)
            }
        }
        isBlockStart = true
    }

    fun write(text: String) {
        if (isBlockStart) {
            val contentStartIndex = text.indexOfFirst(0) { !it.isWhitespace() }
            if (contentStartIndex != -1) {
                writePendingNewLines(0)
                callbacks?.onWriteContentStart()
                output.append(text, contentStartIndex, text.length)
            }
            isBlockStart = false
        } else {
            writePendingNewLines(0)
            callbacks?.onWriteContentStart()
            output.append(text)
        }
    }

    fun writePreformatted(text: String) {
        writePendingNewLines(0)
        callbacks?.onWriteContentStart()
        output.append(text)
        isBlockStart = false
    }

    fun writeLineBreak() {
        writePendingNewLines(1)
    }

    private inline fun CharSequence.indexOfFirst(startIndex: Int, predicate: (Char) -> Boolean): Int {
        for (index in startIndex..<length) {
            if (predicate(this[index])) {
                return index
            }
        }
        return -1
    }

    private fun writePendingNewLines(resetNewLineCount: Int) {
        pendingNewLineCount.let { newLineCount ->
            if (newLineCount != resetNewLineCount) {
                pendingNewLineCount = resetNewLineCount
            }
            if (newLineCount > 0) {
                repeat(callbacks?.onWriteNewLines(newLineCount) ?: newLineCount) {
                    output.append('\n')
                }
            }
        }
    }
}
