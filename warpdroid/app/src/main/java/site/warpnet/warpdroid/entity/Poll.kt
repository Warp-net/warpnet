/*
 * Warpdroid - a Warpnet Android client.
 * Copyright (C) 2026 Warpdroid contributors.
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package site.warpnet.warpdroid.entity

import androidx.compose.runtime.Immutable
import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass
import java.util.Date

@Immutable
@JsonClass(generateAdapter = true)
data class Poll(
    val options: List<PollOption>,
    @Json(name = "expires_at") val expiresAt: Date? = null,
    @Json(name = "total_votes") val totalVotes: Long = 0,
    @Json(name = "voted_option") val votedOption: Int? = null,
) {
    val expired: Boolean
        get() = expiresAt != null && !expiresAt.after(Date())

    val voted: Boolean
        get() = votedOption != null

    val showResults: Boolean
        get() = voted || expired

    fun percent(index: Int): Int {
        if (totalVotes <= 0L) return 0
        val votes = options.getOrNull(index)?.votesCount ?: return 0
        return ((votes * 100.0) / totalVotes).toInt()
    }

    fun withResults(votes: List<Long>, total: Long, voted: Int?): Poll = copy(
        options = options.mapIndexed { index, option ->
            option.copy(votesCount = votes.getOrElse(index) { 0L })
        },
        totalVotes = total,
        votedOption = voted?.takeIf { it in options.indices },
    )
}

@Immutable
@JsonClass(generateAdapter = true)
data class PollOption(
    val title: String,
    @Json(name = "votes_count") val votesCount: Long = 0,
)
