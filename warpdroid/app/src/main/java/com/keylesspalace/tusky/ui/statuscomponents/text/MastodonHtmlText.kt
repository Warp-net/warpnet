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

package com.keylesspalace.tusky.ui.statuscomponents.text

import androidx.compose.material3.MaterialTheme.colorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.LinkInteractionListener
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withLink
import com.keylesspalace.tusky.entity.Emoji
import com.keylesspalace.tusky.entity.HashTag
import com.keylesspalace.tusky.entity.Status
import com.keylesspalace.tusky.ui.statuscomponents.text.background.QuotePainter
import com.keylesspalace.tusky.ui.statuscomponents.text.html.AnnotatedStringHtmlHandler
import com.keylesspalace.tusky.ui.statuscomponents.text.html.FilteringHtmlHandler
import com.keylesspalace.tusky.ui.statuscomponents.text.html.KtXmlParser
import com.keylesspalace.tusky.ui.tuskyColors
import com.keylesspalace.tusky.util.HASHTAG_EXPRESSION
import com.keylesspalace.tusky.util.normalizeToASCII
import com.keylesspalace.tusky.viewdata.StatusViewData
import java.util.regex.Pattern

/**
 * Parses Status content into an [AnnotatedString] and optionally splits off trailing hashtags.
 * Supports custom emojis.
 * The Text where the parsed content is displayed must have [QuotePainter] and inlineContent for emojis and link icon set.
 * @param splitOffTrailingHashtags true to remove trailing hashtags from the parsed content
 * @return The parsed content and the list of hashtags that are not part of the content, if [splitOffTrailingHashtags] was true
 */
@Composable
fun mastodonHtmlText(
    status: StatusViewData.Concrete,
    onMentionClick: (userid: String) -> Unit,
    onHashtagClick: (tag: String) -> Unit,
    onUrlClick: (url: String) -> Unit,
    splitOffTrailingHashtags: Boolean,
    linkStyles: TextLinkStyles = linkStyles()
): Pair<AnnotatedString, List<String>> {
    val actionable = status.actionable
    val quoteColor = tuskyColors.tertiaryTextColor

    return remember(actionable.content, status.translation, actionable.quote, linkStyles, quoteColor) {
        val html = htmlToAnnotatedString(
            html = status.translation?.data?.content ?: actionable.content,
            removeInlineQuotes = status.quote != null,
            linkStyles = linkStyles,
            quoteColor = quoteColor,
            linkInteractionListener = { link ->
                if (link is LinkAnnotation.Url) {
                    onUrlClick(link.url)
                }
            },
            emojis = actionable.emojis
        )

        val mappedHtml = html.mapAnnotations { annotationRange ->
            val annotation = annotationRange.item
            if (annotation is LinkAnnotation.Url) {
                val linkText = html.text.substring(annotationRange.start, annotationRange.end).trim()
                if (linkText.startsWith("@")) {
                    val mention = actionable.mentions.find { mention -> mention.url == annotation.url }
                    if (mention != null) {
                        return@mapAnnotations AnnotatedString.Range(
                            item = LinkAnnotation.Clickable(
                                tag = mention.username,
                                styles = linkStyles,
                                linkInteractionListener = {
                                    onMentionClick(mention.id)
                                }
                            ),
                            start = annotationRange.start,
                            end = annotationRange.end
                        )
                    }
                } else if (linkText.startsWith("#")) {
                    return@mapAnnotations AnnotatedString.Range(
                        item = LinkAnnotation.Clickable(
                            tag = linkText,
                            styles = linkStyles,
                            linkInteractionListener = {
                                onHashtagClick(linkText.substring(1, linkText.length))
                            }
                        ),
                        start = annotationRange.start,
                        end = annotationRange.end
                    )
                }
            }

            annotationRange
        }

        if (splitOffTrailingHashtags) {
            getTrailingHashtags(mappedHtml, actionable.tags)
        } else {
            mappedHtml to emptyList()
        }
    }
}

@Composable
fun List<Status.Mention>.toAnnotatedString(
    onMentionClick: (accountId: String) -> Unit,
    linkStyles: TextLinkStyles = linkStyles()
): AnnotatedString {
    return remember(this, linkStyles) {
        buildAnnotatedString {
            forEach { mention ->
                withLink(
                    LinkAnnotation.Clickable(
                        tag = mention.url,
                        styles = linkStyles,
                        linkInteractionListener = {
                            onMentionClick(mention.id)
                        }
                    )
                ) {
                    append("@")
                    append(mention.username)
                }
                append(" ")
            }
        }
    }
}

fun htmlToAnnotatedString(
    html: String,
    removeInlineQuotes: Boolean,
    linkStyles: TextLinkStyles,
    quoteColor: Color,
    linkInteractionListener: LinkInteractionListener? = null,
    emojis: List<Emoji>
): AnnotatedString {
    val builder = AnnotatedString.Builder()
    KtXmlParser(html.iterator()).parse(
        if (removeInlineQuotes) {
            FilteringHtmlHandler(
                AnnotatedStringHtmlHandler(builder, linkStyles, quoteColor, linkInteractionListener, emojis)
            )
        } else {
            AnnotatedStringHtmlHandler(builder, linkStyles, quoteColor, linkInteractionListener, emojis)
        }
    )

    return builder.toAnnotatedString()
}

private val hashtagWithHashPattern = "^#$HASHTAG_EXPRESSION$".toPattern()
private val whitespacePattern = Regex("""\s+""")

/**
 * Find the "trailing" hashtags in an AnnotatedString.
 * These are hashtags in lines consisting *only* of hashtags at the end of the post.
 * @param content The [AnnotatedString] to search for hashtags.
 * @param serverTags Tags from the server. Sometimes these contain additional tags not in the content.
 * @param hashtagPattern The [Pattern] to use for finding hashtags. Only for testing.
 * @return The content without trailing hashtags and a list of the trailing hashtags.
 */
internal fun getTrailingHashtags(
    content: AnnotatedString,
    serverTags: List<HashTag>,
    hashtagPattern: Pattern = hashtagWithHashPattern
): Pair<AnnotatedString, List<String>> {
    // split() instead of lines() because we need to be able to account for the length of the removed delimiter
    val trailingContentLength = content.split('\r', '\n').asReversed().takeWhile { line ->
        line.splitToSequence(whitespacePattern).all { it.isBlank() || hashtagPattern.matcher(it).matches() }
    }.sumOf { it.length + 1 } // length + 1 to include the stripped line ending character

    val trailingContentOffset = (content.length - trailingContentLength).coerceAtLeast(0)

    val inlineHashtags = content.getHashtagsInRange(0, trailingContentOffset)

    val trailingHashtags = content.getHashtagsInRange(trailingContentOffset, content.length)

    val missingServerTags = serverTags.filterNot { serverTag ->
        inlineHashtags.any { inlineTag -> serverTag.name.equals(normalizeToASCII(inlineTag), ignoreCase = true) } ||
            trailingHashtags.any { inlineTag -> serverTag.name.equals(normalizeToASCII(inlineTag), ignoreCase = true) }
    }.map { tag -> tag.name }

    return content.subSequence(0, trailingContentOffset) to trailingHashtags + missingServerTags
}

/** returns the list of hashtags (without #) that are found in the specified range of the AnnotatedString.
 * @param startIndex The start index (inclusive), must be > 0.
 * @param endIndex The end index (exclusive), must be less than the length of the AnnotatedString.
 * */
private fun AnnotatedString.getHashtagsInRange(startIndex: Int, endIndex: Int): List<String> {
    return getLinkAnnotations(startIndex, endIndex)
        .mapNotNull { annotation ->
            val annotationContent = subSequence(annotation.start, annotation.end).trim()
            if (annotationContent.firstOrNull() == '#') {
                annotationContent.drop(1).toString()
            } else {
                null
            }
        }
}

@Composable
fun linkStyles(): TextLinkStyles {
    val primaryColor = colorScheme.primary

    val activeLinkStyle = SpanStyle(color = primaryColor, background = primaryColor.copy(alpha = 0.25f))
    return TextLinkStyles(
        style = SpanStyle(color = primaryColor),
        focusedStyle = activeLinkStyle,
        hoveredStyle = activeLinkStyle,
        pressedStyle = activeLinkStyle
    )
}
