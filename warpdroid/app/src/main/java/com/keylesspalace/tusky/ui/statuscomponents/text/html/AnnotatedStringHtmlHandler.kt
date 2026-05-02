package com.keylesspalace.tusky.ui.statuscomponents.text.html

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

import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.LinkInteractionListener
import androidx.compose.ui.text.ParagraphStyle
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.BaselineShift
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextIndent
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.util.getDomain

internal class AnnotatedStringHtmlHandler(
    private val builder: AnnotatedString.Builder,
    private val linkStyles: TextLinkStyles,
    private val quoteColor: Color,
    private val linkInteractionListener: LinkInteractionListener?,
    private val emojis: List<Emoji>
) : HtmlHandler {
    private val textWriter = HtmlTextWriter(
        builder,
        object : HtmlTextWriter.Callbacks {
            private var consumedNewLineIndex = -1

            override fun onWriteNewLines(newLineCount: Int): Int {
                val currentIndex = builder.length
                if (currentIndex != consumedNewLineIndex) {
                    val startIndex = paragraphStartIndex
                    if (currentIndex == startIndex || (startIndex < 0 && currentIndex == paragraphEndIndex)) {
                        // Paragraph style will automatically add a single new line at each boundary
                        consumedNewLineIndex = currentIndex
                        return newLineCount - 1
                    }
                }
                return newLineCount
            }

            override fun onWriteContentStart() {
                pushPendingSpanStyles()
                pushPendingQuoteAnnotations()
            }
        }
    )
    private val pendingSpanStyles = mutableListOf<SpanStyle>()
    private var pendingQuoteAnnotationCount: Int = 0
    private var listLevel = 0

    // A negative index means the list is unordered
    private var listIndexes: IntArray = EMPTY_LIST_INDEXES
    private var preformattedLevel = 0
    private var boldLevel = 0
    private var skippedTagsLevel = 0
    private var blockLevel = 0
    private var blockIndentLevel = 0
    private var paragraphStartIndex = -1
    private var paragraphEndIndex = -1
    private var currentLink = ""
    private var currentLinkText = ""

    private fun pushPendingSpanStyles() {
        val size = pendingSpanStyles.size
        if (size != 0) {
            for (i in 0..<size) {
                builder.pushStyle(pendingSpanStyles[i])
            }
            pendingSpanStyles.clear()
        }
    }

    private fun pushPendingQuoteAnnotations() {
        repeat(pendingQuoteAnnotationCount) {
            builder.pushStringAnnotation(QUOTE_ANNOTATION, blockIndentLevel.toString())
        }
        pendingQuoteAnnotationCount = 0
    }

    override fun onOpenTag(name: String, attributes: (String) -> String?) {
        when (name) {
            "br" -> handleLineBreakStart()
            "hr" -> handleHorizontalRuleStart()
            "p" -> handleBlockStart(2, 0)
            "blockquote" -> handleQuoteStart()
            "div", "header", "footer", "main", "nav", "aside", "section", "article",
            "address", "figure", "figcaption",
            "video", "audio" -> handleBlockStart(1, 0)
            "ul", "dl" -> handleListStart(-1)
            "ol" -> handleListStart(1)
            "li" -> handleListItemStart()
            "dt" -> handleDefinitionTermStart()
            "dd" -> handleDefinitionDetailStart()
            "pre" -> handlePreStart()
            "strong", "b" -> handleBoldStart()
            "em", "cite", "dfn", "i" ->
                handleSpanStyleStart(SpanStyle(fontStyle = FontStyle.Italic))
            "big" -> handleSpanStyleStart(SpanStyle(fontSize = 1.25.em))
            "small" -> handleSpanStyleStart(SpanStyle(fontSize = 0.8.em))
            "tt", "code" -> handleSpanStyleStart(
                SpanStyle(fontFamily = FontFamily.Monospace)
            )
            "a" -> handleAnchorStart(attributes("href").orEmpty())

            "u" -> handleSpanStyleStart(
                SpanStyle(textDecoration = TextDecoration.Underline)
            )
            "del", "s", "strike" ->
                handleSpanStyleStart(
                    SpanStyle(textDecoration = TextDecoration.LineThrough)
                )
            "sup" -> handleSpanStyleStart(
                SpanStyle(baselineShift = BaselineShift.Superscript)
            )
            "sub" -> handleSpanStyleStart(
                SpanStyle(baselineShift = BaselineShift.Subscript)
            )
            "span" -> { }
            "h1", "h2", "h3", "h4", "h5", "h6" -> handleHeadingStart(name)
            "script", "head", "table", "form", "fieldset" -> handleSkippedTagStart()
        }
    }

    private fun handleLineBreakStart() {
        textWriter.writeLineBreak()
    }

    private fun handleHorizontalRuleStart() {
        textWriter.markBlockBoundary(2)
    }

    /**
     * Add a pending paragraph, if any, and return true if it was added.
     */
    private fun addPendingParagraph(currentIndex: Int): Boolean {
        // Close current paragraph, if any
        paragraphStartIndex.let { startIndex ->
            if (startIndex in 0..<currentIndex) {
                val indentSize = blockIndentLevel.sp
                builder.addStyle(
                    style = ParagraphStyle(
                        textIndent = TextIndent(
                            firstLine = indentSize,
                            restLine = indentSize
                        )
                    ),
                    start = startIndex,
                    end = currentIndex
                )
                return true
            }
        }
        return false
    }

    private fun handleBlockStart(prefixNewLineCount: Int, indent: Int) {
        var level = blockLevel
        if (level >= 0) {
            val currentIndex = builder.length
            addPendingParagraph(currentIndex)
            paragraphStartIndex = currentIndex
            level++
            blockLevel = level
            blockIndentLevel += indent
        }
        textWriter.markBlockBoundary(prefixNewLineCount)
    }

    private fun handleQuoteStart() {
        handleBlockStart(2, QUOTE_INDENT)
        pendingQuoteAnnotationCount++
        pendingSpanStyles.add(SpanStyle(color = quoteColor))
    }

    private fun handleListStart(initialIndex: Int) {
        val currentListLevel = listLevel
        handleBlockStart(if (currentListLevel == 0) 2 else 1, 0)
        val listIndexesSize = listIndexes.size
        // Ensure listIndexes capacity
        if (currentListLevel == listIndexesSize) {
            listIndexes = if (listIndexesSize == 0) {
                IntArray(INITIAL_LIST_INDEXES_SIZE)
            } else {
                listIndexes.copyOf(listIndexesSize * 2)
            }
        }
        listIndexes[currentListLevel] = initialIndex
        listLevel = currentListLevel + 1
    }

    private fun handleListItemStart() {
        val currentListLevel = listLevel
        handleBlockStart(1, if (currentListLevel > 1) LIST_INDENT else 0)

        val itemIndex = if (currentListLevel == 0) {
            -1
        } else {
            listIndexes[currentListLevel - 1]
        }
        if (itemIndex < 0) {
            textWriter.write(getBulletString(currentListLevel))
        } else {
            textWriter.write(itemIndex.toString())
            textWriter.write(". ")
            listIndexes[currentListLevel - 1] = itemIndex + 1
        }
    }

    private fun getBulletString(listLevel: Int): String = when (listLevel) {
        0, 1 -> "• "
        2 -> "◦ "
        3 -> "▪ "
        else -> "▫ "
    }

    private fun handleDefinitionTermStart() {
        handleBlockStart(1, 0)
    }

    private fun handleDefinitionDetailStart() {
        handleBlockStart(1, 8)
    }

    private fun handlePreStart() {
        handleBlockStart(2, 0)
        handleSpanStyleStart(SpanStyle(fontFamily = FontFamily.Monospace))
        preformattedLevel++
    }

    private fun incrementBoldLevel(): FontWeight {
        val level = boldLevel + 1
        boldLevel = level
        return if (level == 1) FontWeight.Bold else FontWeight.Black
    }

    private fun handleBoldStart() {
        handleSpanStyleStart(SpanStyle(fontWeight = incrementBoldLevel()))
    }

    private fun handleSpanStyleStart(style: SpanStyle) {
        // Defer pushing the span style until the actual content is about to be written
        pendingSpanStyles.add(style)
    }

    private fun handleAnchorStart(url: String) {
        currentLink = url
        builder.pushLink(
            LinkAnnotation.Url(
                url = url,
                styles = linkStyles,
                linkInteractionListener = linkInteractionListener
            )
        )
    }

    private fun handleHeadingStart(name: String) {
        handleBlockStart(2, 0)
        val level = name[1].digitToInt()
        handleSpanStyleStart(
            SpanStyle(
                fontSize = HEADING_SIZES[level - 1].em,
                fontWeight = incrementBoldLevel()
            )
        )
    }

    private fun handleSkippedTagStart() {
        skippedTagsLevel++
    }

    override fun onCloseTag(name: String) {
        when (name) {
            "br",
            "hr" -> {}
            "p" -> handleBlockEnd(2, 0)
            "blockquote" -> handleQuoteEnd()
            "div", "header", "footer", "main", "nav", "aside", "section", "article",
            "address", "figure", "figcaption",
            "video", "audio" -> handleBlockEnd(1, 0)
            "ul", "dl",
            "ol" -> handleListEnd()
            "li" -> handleListItemEnd()
            "dt" -> handleDefinitionTermEnd()
            "dd" -> handleDefinitionDetailEnd()
            "pre" -> handlePreEnd()
            "strong", "b" -> handleBoldEnd()
            "em", "cite", "dfn", "i",
            "big",
            "small",
            "tt", "code",
            "u",
            "del", "s", "strike",
            "sup",
            "sub" -> handleSpanStyleEnd()
            "span" -> { }
            "a" -> handleAnchorEnd()
            "h1", "h2", "h3", "h4", "h5", "h6" -> handleHeadingEnd()
            "script", "head", "table", "form", "fieldset" -> handleSkippedTagEnd()
        }
    }

    private fun handleBlockEnd(suffixNewLineCount: Int, indent: Int) {
        var level = blockLevel
        if (level >= 0) {
            val currentIndex = builder.length
            // Paragraph will only be added if non-empty
            if (addPendingParagraph(currentIndex)) {
                paragraphEndIndex = currentIndex
            }
            level--
            blockLevel = level

            blockIndentLevel -= indent

            // Start a new paragraph automatically unless we're back at level 0
            paragraphStartIndex = if (level == 0) -1 else currentIndex
        }
        textWriter.markBlockBoundary(suffixNewLineCount)
    }

    private fun handleQuoteEnd() {
        handleSpanStyleEnd()
        if (pendingQuoteAnnotationCount == 0) {
            builder.pop()
        } else {
            pendingQuoteAnnotationCount--
        }
        handleBlockEnd(2, QUOTE_INDENT)
    }

    private fun handleListEnd() {
        listLevel--
        handleBlockEnd(if (listLevel == 0) 2 else 1, 0)
    }

    private fun handleListItemEnd() {
        handleBlockEnd(1, if (listLevel > 1) LIST_INDENT else 0)
    }

    private fun handleDefinitionTermEnd() {
        handleBlockEnd(1, 0)
    }

    private fun handleDefinitionDetailEnd() {
        handleBlockEnd(1, 12)
    }

    private fun handlePreEnd() {
        preformattedLevel--
        handleSpanStyleEnd()
        handleBlockEnd(2, 0)
    }

    private fun decrementBoldLevel() {
        boldLevel--
    }

    private fun handleBoldEnd() {
        handleSpanStyleEnd()
        decrementBoldLevel()
    }

    private fun handleSpanStyleEnd() {
        val size = pendingSpanStyles.size
        if (size == 0) {
            builder.pop()
        } else {
            pendingSpanStyles.removeAt(size - 1)
        }
    }

    private fun handleAnchorEnd() {
        if (currentLink.isNotEmpty() &&
            !currentLinkText.startsWith("#") &&
            !currentLinkText.startsWith("@") &&
            currentLinkText != currentLink
        ) {
            val linkDomain = getDomain(currentLink)
            if (currentLinkText != linkDomain) {
                writeText(" (")
                builder.appendInlineContent(LINK_ICON_ID, "🔗")
                writeText(" ")
                writeText(linkDomain)
                writeText(")")
            }
        }

        currentLink = ""
        currentLinkText = ""

        builder.pop()
    }

    private fun handleHeadingEnd() {
        handleSpanStyleEnd()
        decrementBoldLevel()
        handleBlockEnd(1, 0)
    }

    private fun handleSkippedTagEnd() {
        skippedTagsLevel--
    }

    override fun onText(text: String) {
        // Skip text inside skipped tags
        if (skippedTagsLevel > 0) {
            return
        }
        var lastWrittenIndex = 0
        var index = 0

        while (index < text.length) {
            if (text[index] == ':') {
                val nextSubstring = text.substring(startIndex = index + 1)
                for (emoji in emojis) {
                    if (nextSubstring.startsWith(emoji.shortcode + ":")) {
                        writeText(text.substring(lastWrittenIndex, index))
                        builder.appendInlineContent(emoji.shortcode, ":${emoji.shortcode}:")
                        index += emoji.shortcode.length + 1
                        lastWrittenIndex = index + 1
                        break
                    }
                }
            }
            index++
        }

        writeText(text.substring(lastWrittenIndex, text.length))

        if (currentLink.isNotEmpty()) {
            currentLinkText += text
        }
    }

    private fun writeText(text: String) {
        if (preformattedLevel == 0) {
            textWriter.write(text)
        } else {
            textWriter.writePreformatted(text)
        }
    }

    companion object {
        const val QUOTE_ANNOTATION = "QUOTE"
        const val LINK_ICON_ID = "LINK"

        private val HEADING_SIZES = floatArrayOf(1.5f, 1.4f, 1.3f, 1.2f, 1.1f, 1f)
        private val EMPTY_LIST_INDEXES = intArrayOf()
        private const val INITIAL_LIST_INDEXES_SIZE = 8

        private const val LIST_INDENT = 16
        private const val QUOTE_INDENT = 8
    }
}
