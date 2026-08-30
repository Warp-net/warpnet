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

    const val MIN_POLL_OPTIONS: Int = 2

    const val MAX_POLL_OPTIONS: Int = 4

    const val MAX_POLL_OPTION_CHARS: Int = 25

    const val MAX_IMAGES_PER_TWEET: Int = 4

    const val MAX_VIDEO_BYTES: Long = 36L * 1024L * 1024L

    val ACCEPTED_VIDEO_MIME_TYPES: Set<String> = setOf(
        "video/mp4",
        "video/quicktime",
        "video/x-m4v",
    )
}
