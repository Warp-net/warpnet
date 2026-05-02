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

import org.kobjects.ktxml.api.EventType
import org.kobjects.ktxml.api.XmlPullParser
import org.kobjects.ktxml.mini.MiniXmlPullParser

internal class KtXmlParser(private val html: CharIterator) : HtmlParser {
    override fun parse(handler: HtmlHandler) {
        val parser: XmlPullParser = MiniXmlPullParser(
            source = html,
            relaxed = true,
            entityResolver = HtmlEntities.entityResolver
        )
        val attributes = { name: String -> parser.getAttributeValue("", name) }
        val tagStack = mutableListOf<String>()

        while (true) {
            when (parser.nextToken()) {
                EventType.START_TAG -> {
                    val lowerCaseName = parser.name.lowercase()
                    handler.onOpenTag(lowerCaseName, attributes)
                    if (lowerCaseName == "br" || lowerCaseName == "hr" || lowerCaseName == "img") {
                        // Special case for unpaired tags: closing event is notified immediately
                        handler.onCloseTag(lowerCaseName)
                        if (parser.isEmptyElementTag) {
                            parser.next()
                        }
                    } else {
                        tagStack.add(lowerCaseName)
                    }
                }

                EventType.END_TAG -> {
                    val name = parser.name
                    if (name.equals("br", ignoreCase = true)) {
                        // A closing BR tag is interpreted as a self-closing BR tag
                        handler.onOpenTag("br", EMPTY_ATTRIBUTES)
                        handler.onCloseTag("br")
                    } else {
                        val stackPosition =
                            tagStack.indexOfLast { it.equals(name, ignoreCase = true) }
                        if (stackPosition != -1) {
                            // Also close all unclosed child tags, if any
                            for (i in tagStack.lastIndex downTo stackPosition) {
                                handler.onCloseTag(tagStack.removeAt(i))
                            }
                        }
                    }
                }

                EventType.TEXT, EventType.ENTITY_REF -> handler.onText(parser.text)
                EventType.IGNORABLE_WHITESPACE -> handler.onText(" ")
                EventType.END_DOCUMENT -> break
                else -> {}
            }
        }

        // Close remaining open tags, if any
        for (i in tagStack.lastIndex downTo 0) {
            handler.onCloseTag(tagStack[i])
        }
    }

    companion object {
        private val EMPTY_ATTRIBUTES: (String) -> String? = { null }
    }
}
