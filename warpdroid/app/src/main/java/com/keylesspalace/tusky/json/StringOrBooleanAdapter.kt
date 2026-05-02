/*
 * Copyright 2024 Tusky Contributors
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
 * see <http://www.gnu.org/licenses>.
 */

package com.keylesspalace.tusky.json

import android.util.Log
import com.squareup.moshi.JsonAdapter
import com.squareup.moshi.JsonReader
import com.squareup.moshi.JsonWriter
import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import java.lang.reflect.Type

/**
 * This adapter tries to read a value as boolean or as a String that encodes a boolean.
 * If both fail, it defaults to false.
 */
class StringOrBooleanAdapter private constructor() : JsonAdapter<Boolean>() {

    override fun fromJson(reader: JsonReader): Boolean? {
        return try {
            when (reader.peek()) {
                JsonReader.Token.NULL -> reader.nextNull()
                JsonReader.Token.BOOLEAN -> reader.nextBoolean()
                JsonReader.Token.STRING -> reader.nextString().toBoolean()
                else -> {
                    reader.skipValue()
                    false
                }
            }
        } catch (e: Exception) {
            Log.w("StringOrBooleanAdapter", "failed to read json", e)
            false
        }
    }

    override fun toJson(writer: JsonWriter, value: Boolean?) {
        if (value == null) {
            writer.nullValue()
        } else {
            writer.value(value)
        }
    }

    companion object {
        val ANNOTATION_FACTORY = object : Factory {
            override fun create(
                type: Type,
                annotations: Set<Annotation>,
                moshi: Moshi
            ): JsonAdapter<*>? {
                Types.nextAnnotations(annotations, StringOrBoolean::class.java) ?: return null
                return StringOrBooleanAdapter()
            }
        }
    }
}
