/* Copyright 2026 Tusky Contributors
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

package com.keylesspalace.tusky.entity

import androidx.compose.runtime.Immutable
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@Immutable
@JsonClass(generateAdapter = true)
data class Quote(
    val state: State,
    @Json(name = "quoted_status") val quotedStatus: Status?
) {

    /** The state of the quote. Unknown values should be treated as unauthorized. */
    @JsonClass(generateAdapter = false)
    enum class State {
        /** The quote has not been acknowledged by the quoted account yet, and requires authorization before being displayed. */
        @Json(name = "pending")
        PENDING,

        /** The quote has been accepted and can be displayed. quoted_status is non-null. */
        @Json(name = "accepted")
        ACCEPTED,

        /** The quote has been explicitly rejected by the quoted account, and cannot be displayed. */
        @Json(name = "rejected")
        REJECTED,

        /** The quote has been previously accepted, but is now revoked, and thus cannot be displayed. */
        @Json(name = "revoked")
        REVOKED,

        /** The quote has been approved, but the quoted post itself has now been deleted. **/
        @Json(name = "deleted")
        DELETED,

        /** The quote has been approved, but cannot be displayed because the user is not authorized to see it. **/
        @Json(name = "unauthorized")
        UNAUTHORIZED,

        /** The quote has been approved, but should not be displayed because the user has blocked the account being quoted. quoted_status is non-null. **/
        @Json(name = "blocked_account")
        BLOCKED_ACCOUNT,

        /** The quote has been approved, but should not be displayed because the user has blocked the domain of the account being quoted. quoted_status is non-null. **/
        @Json(name = "blocked_domain")
        BLOCKED_DOMAIN,

        /** The quote has been approved, but should not be displayed because the user has muted the the account being quoted. quoted_status is non-null. **/
        @Json(name = "muted_account")
        MUTED_ACCOUNT
    }
}
