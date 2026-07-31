// SPDX-License-Identifier: AGPL-3.0-or-later
// Detects YouTube links in tweet text so they can be previewed and played
// inline. Nothing here contacts YouTube: the caller renders a local facade
// and only builds an embed URL once the user asks to play.

// A YouTube video id is exactly 11 characters from the URL-safe alphabet.
// The trailing guard stops an 11-character prefix of some longer token from
// matching.
const ID = "([A-Za-z0-9_-]{11})(?![A-Za-z0-9_-])";

const YOUTUBE_PATTERNS = [
    new RegExp(`(?:https?://)?(?:www\\.|m\\.|music\\.)?youtube\\.com/watch\\?(?:[^\\s]*&)?v=${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.)?youtu\\.be/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/shorts/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/embed/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/live/${ID}`, "i"),
];

// extractYoutubeId returns the id of the first YouTube link in text, or null.
export function extractYoutubeId(text) {
    if (!text || typeof text !== "string") return null;
    for (const pattern of YOUTUBE_PATTERNS) {
        const match = text.match(pattern);
        if (match && match[1]) return match[1];
    }
    return null;
}

// youtubeEmbedUrl builds a nocookie player URL. Only called after the user
// clicks play, so no request reaches Google before that.
export function youtubeEmbedUrl(videoId) {
    return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(videoId)}?autoplay=1&rel=0`;
}

export function youtubeWatchUrl(videoId) {
    return `https://www.youtube.com/watch?v=${encodeURIComponent(videoId)}`;
}
