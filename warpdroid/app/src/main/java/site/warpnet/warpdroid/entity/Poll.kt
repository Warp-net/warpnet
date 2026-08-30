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

/**
 * A poll attached to a tweet.
 *
 * Warpnet polls are single-choice and final: the node stores one option per
 * voter and rejects a second vote, so there is no "change your mind" path and
 * no multiple-choice variant. [options] and [expiresAt] travel with the tweet;
 * the tallies are a separate read that the repository folds in, so a poll
 * whose results haven't been fetched yet has zeroed counts and a null
 * [votedOption] rather than a missing poll.
 */
@Immutable
@JsonClass(generateAdapter = true)
data class Poll(
    val options: List<PollOption>,
    @Json(name = "expires_at") val expiresAt: Date? = null,
    @Json(name = "total_votes") val totalVotes: Long = 0,
    /** The option this account picked, null until they vote. */
    @Json(name = "voted_option") val votedOption: Int? = null,
) {
    val expired: Boolean
        get() = expiresAt != null && !expiresAt.after(Date())

    val voted: Boolean
        get() = votedOption != null

    /**
     * Same rule as the desktop frontend: the tally stays hidden until this
     * account has had its say or the poll has closed, so early results can't
     * steer later voters.
     */
    val showResults: Boolean
        get() = voted || expired

    /** Share of [totalVotes] held by [index], as a whole percentage. */
    fun percent(index: Int): Int {
        if (totalVotes <= 0L) return 0
        val votes = options.getOrNull(index)?.votesCount ?: return 0
        return ((votes * 100.0) / totalVotes).toInt()
    }

    /**
     * Apply a tally read onto this definition. [votes] is the count per
     * option in option order; a short list leaves the remaining options at
     * zero rather than dropping them, so the choices a voter sees always
     * match the ones the author wrote.
     */
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
