/* Warpnet - Decentralized Social Network */
package site.warpnet.transport

/**
 * Static limits Warpnet enforces on tweet content.
 *
 * Warpnet has no "instance" concept and no `GET /api/v1/instance` endpoint:
 * every node enforces the same hard-coded constants from the
 * `core/handler` package on the backend, so the client doesn't need to
 * probe these at runtime. Mirror them here so compose-screen char
 * counters and validators have a single source of truth.
 */
object WarpnetLimits {
    /** Mirrors `tweetCharLimit` in core/handler/tweet.go. */
    const val MAX_TWEET_CHARS: Int = 280
}
