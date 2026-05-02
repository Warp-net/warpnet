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
 * Implement this interface to provide your own HTML parser feeding the handler.
 */
public interface HtmlParser {
    public fun parse(handler: HtmlHandler)
}
