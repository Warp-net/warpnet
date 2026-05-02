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

package com.keylesspalace.tusky.ui.statuscomponents.text.background

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.ui.statuscomponents.text.html.AnnotatedStringHtmlHandler.Companion.QUOTE_ANNOTATION

class QuotePainter(val color: Color) : TextBackgroundPainter() {

    override fun drawInstructions(layoutResult: TextLayoutResult): BackgroundDrawInstructions {
        val text = layoutResult.layoutInput.text
        val annotations = text.getStringAnnotations(QUOTE_ANNOTATION, 0, text.length)

        return BackgroundDrawInstructions {
            annotations.forEach { annotation ->
                val quoteStartLine = try {
                    layoutResult.getLineForOffset(annotation.start)
                } catch (_: IllegalArgumentException) {
                    // can happen if the line is currently not visible - nothing to draw here
                    return@forEach
                }
                if (quoteStartLine >= layoutResult.lineCount) {
                    return@forEach
                }

                val quoteEndLine = try {
                    layoutResult.getLineForOffset(annotation.end - 1)
                } catch (_: IllegalArgumentException) {
                    // can happen if the line is currently not visible - fallback to the last visible line
                    layoutResult.lineCount - 1
                }.coerceAtMost(layoutResult.lineCount - 1)

                val quoteStart = layoutResult.getLineTop(quoteStartLine)
                val quoteEnd = layoutResult.getLineBottom(quoteEndLine)

                // the annotation item is the indent of the text, the indicator bar needs to be 8sp to the side
                val quoteIndent = (annotation.item.toInt() - 8).sp.toPx()

                val indicatorWidth = 3.sp.toPx()

                drawRect(color, Offset(quoteIndent, quoteStart), Size(indicatorWidth, quoteEnd - quoteStart))
            }
        }
    }
}
