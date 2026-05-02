package com.keylesspalace.tusky.util

import android.content.Context
import android.text.Spannable
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.style.CharacterStyle
import android.text.style.DynamicDrawableSpan
import android.text.style.ForegroundColorSpan
import android.text.style.ImageSpan
import android.text.style.URLSpan
import androidx.appcompat.content.res.AppCompatResources
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.material3.Icon
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.unit.sp
import com.keylesspalace.tusky.R
import com.keylesspalace.tusky.ui.statuscomponents.text.INLINE_CONTENT_TAG
import com.keylesspalace.tusky.ui.statuscomponents.text.htmlToAnnotatedString
import com.keylesspalace.tusky.util.twittertext.Regex
import java.util.regex.Pattern

/**
 * @see <a href="https://github.com/tootsuite/mastodon/blob/master/app/models/tag.rb">
 *     Tag#HASHTAG_RE</a>.
 */
private const val HASHTAG_SEPARATORS = "_\\u00B7\\u30FB\\u200c"
internal const val TAG_PATTERN_STRING = "(?<![=/)\\p{Alnum}])(#(([\\w_][\\w$HASHTAG_SEPARATORS]*[\\p{Alpha}$HASHTAG_SEPARATORS][\\w$HASHTAG_SEPARATORS]*[\\w_])|([\\w_]*[\\p{Alpha}][\\w_]*)))"
private val TAG_PATTERN = TAG_PATTERN_STRING.toPattern(Pattern.CASE_INSENSITIVE)

/**
 * @see <a href="https://github.com/tootsuite/mastodon/blob/master/app/models/account.rb">
 *     Account#MENTION_RE</a>
 */
private const val USERNAME_PATTERN_STRING = "[a-z0-9_]+([a-z0-9_.-]+[a-z0-9_]+)?"
internal const val MENTION_PATTERN_STRING = "(?<![=/\\w])(@($USERNAME_PATTERN_STRING)(?:@[\\w.-]+[\\w]+)?)"
private val MENTION_PATTERN = MENTION_PATTERN_STRING.toPattern(Pattern.CASE_INSENSITIVE)

private val VALID_URL_PATTERN = Regex.VALID_URL_PATTERN_STRING.toPattern(Pattern.CASE_INSENSITIVE)

private val spanClasses = listOf(ForegroundColorSpan::class.java, URLSpan::class.java)

// url must come first, it may contain the other patterns
val defaultFinders = listOf(
    PatternFinder("http", FoundMatchType.HTTP_URL, VALID_URL_PATTERN),
    PatternFinder("#", FoundMatchType.TAG, TAG_PATTERN),
    PatternFinder("@", FoundMatchType.MENTION, MENTION_PATTERN)
)

enum class FoundMatchType {
    HTTP_URL,
    HTTPS_URL,
    TAG,
    MENTION
}

class PatternFinder(
    val searchString: String,
    val type: FoundMatchType,
    val pattern: Pattern
)

/**
 * Takes text containing mentions and hashtags and urls and makes them the given colour.
 * @param finders The finders to use. This is here so they can be overridden from unit tests.
 */
fun Spannable.highlightSpans(colour: Int, finders: List<PatternFinder> = defaultFinders) {
    // Strip all existing colour spans.
    for (spanClass in spanClasses) {
        clearSpans(spanClass)
    }

    for (finder in finders) {
        // before running the regular expression, check if there is even a chance of it finding something
        if (this.contains(finder.searchString, ignoreCase = true)) {
            val matcher = finder.pattern.matcher(this)

            while (matcher.find()) {
                // we found a match
                val start = matcher.start(1)

                val end = matcher.end(1)

                // only add a span if there is no other one yet (e.g. the #anchor part of an url might match as hashtag, but must be ignored)
                if (this.getSpans(start, end, URLSpan::class.java).isEmpty()) {
                    this.setSpan(
                        getSpan(finder.type, this, colour, start, end),
                        start,
                        end,
                        Spanned.SPAN_INCLUSIVE_EXCLUSIVE
                    )
                }
            }
        }
    }
}

private fun <T> Spannable.clearSpans(spanClass: Class<T>) {
    for (span in getSpans(0, length, spanClass)) {
        removeSpan(span)
    }
}

private val iconNameMapping: Map<String, Int> = mapOf(
    "{{home}}" to R.drawable.ic_home_24dp,
    "{{mail}}" to R.drawable.ic_mail_24dp,
    "{{group}}" to R.drawable.ic_group_24dp,
    "{{search}}" to R.drawable.ic_search_24dp,
    "{{manage_accounts}}" to R.drawable.ic_manage_accounts_24dp,
    "{{chevron_right}}" to R.drawable.ic_chevron_right_24dp,
    "{{public}}" to R.drawable.ic_public_24dp,
    "{{edit}}" to R.drawable.ic_edit_24dp_filled
)

/**
 * Replaces text of the form {{icon_name}} with their spanned counterparts (ImageSpan). Supported icon names are above.
 */
fun addDrawables(text: CharSequence, color: Int, size: Int, context: Context): Spannable {
    val builder = SpannableStringBuilder(text)

    iconNameMapping.forEach { (iconName, icon) ->
        var index = 0
        while (index < text.length - iconName.length && index != -1) {
            index = text.indexOf(iconName, index)

            if (index != -1) {
                val drawable = AppCompatResources.getDrawable(context, icon)!!
                drawable.setBounds(0, 0, size, size)
                drawable.setTint(color)
                builder.setSpan(
                    ImageSpan(drawable, DynamicDrawableSpan.ALIGN_CENTER),
                    index,
                    index + iconName.length,
                    Spannable.SPAN_EXCLUSIVE_EXCLUSIVE
                )
                index += iconName.length
            }
        }
    }

    return builder
}

fun addIconAnnotations(text: String): AnnotatedString {
    val textWithStyling = htmlToAnnotatedString(
        html = text,
        removeInlineQuotes = false,
        linkStyles = TextLinkStyles(),
        quoteColor = Color.Unspecified,
        linkInteractionListener = null,
        emojis = emptyList()
    )
    return buildAnnotatedString {
        append(textWithStyling)

        iconNameMapping.forEach { (iconName, _) ->
            var index = 0
            while (index < textWithStyling.length - iconName.length && index != -1) {
                index = textWithStyling.indexOf(iconName, index)

                if (index != -1) {
                    addStringAnnotation(
                        INLINE_CONTENT_TAG,
                        iconName,
                        index,
                        index + iconName.length
                    )
                    index += iconName.length
                }
            }
        }
    }
}

fun iconInlineContent(color: Color): Map<String, InlineTextContent> {
    return iconNameMapping.asIterable()
        .associate { (iconName, icon) ->
            iconName to InlineTextContent(
                placeholder = Placeholder(
                    width = 22.sp,
                    height = 22.sp,
                    placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter
                ),
                children = {
                    Icon(
                        painter = painterResource(icon),
                        modifier = Modifier.fillMaxSize(),
                        tint = color,
                        contentDescription = null,
                    )
                }
            )
        }
}

private fun getSpan(
    matchType: FoundMatchType,
    string: CharSequence,
    colour: Int,
    start: Int,
    end: Int
): CharacterStyle {
    return when (matchType) {
        FoundMatchType.HTTP_URL, FoundMatchType.HTTPS_URL -> NoUnderlineURLSpan(string.substring(start, end))
        FoundMatchType.MENTION -> MentionSpan(string.substring(start, end))
        else -> ForegroundColorSpan(colour)
    }
}
