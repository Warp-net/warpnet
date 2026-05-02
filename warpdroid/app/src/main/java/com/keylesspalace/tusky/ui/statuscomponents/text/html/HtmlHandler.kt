/*
 * Copyright (C) 2023 Christophe Beyls
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

/**
 * Custom parser implementations call methods of this interface to signal parsing events.
 */
public interface HtmlHandler {
    /**
     * Each opening tag must have a following matching closing tag.
     * All tag names must be lower case.
     */
    public fun onOpenTag(name: String, attributes: (String) -> String?)

    /**
     * Each closing tag must have a preceding matching opening tag.
     * Tags must be closed in the exact reverse order they were opened.
     * All tag names must be lower case.
     */
    public fun onCloseTag(name: String)

    /**
     * HTML entities must be decoded.
     * This method may be called multiple times for a single contiguous text block.
     */
    public fun onText(text: String)
}
