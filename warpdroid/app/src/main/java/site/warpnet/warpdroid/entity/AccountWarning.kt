/* Copyright 2025 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.entity

import androidx.annotation.StringRes
import androidx.compose.runtime.Immutable
import site.warpnet.warpdroid.R
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@Immutable
@JsonClass(generateAdapter = true)
data class AccountWarning(
    val id: String,
    val action: Action
) {

    @JsonClass(generateAdapter = false)
    enum class Action(@StringRes val text: Int) {
        @Json(name = "none")
        NONE(R.string.moderation_warning_action_none),

        @Json(name = "disable")
        DISABLE(R.string.moderation_warning_action_disable),

        @Json(name = "mark_statuses_as_sensitive")
        MARK_STATUSES_AS_SENSITIVE(R.string.moderation_warning_action_mark_statuses_as_sensitive),

        @Json(name = "delete_statuses")
        DELETE_STATUSES(R.string.moderation_warning_action_delete_statuses),

        @Json(name = "sensitive")
        SENSITIVE(R.string.moderation_warning_action_sensitive),

        @Json(name = "silence")
        SILENCE(R.string.moderation_warning_action_silence),

        @Json(name = "suspend")
        SUSPEND(R.string.moderation_warning_action_suspend),
    }
}
