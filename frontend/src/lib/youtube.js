// SPDX-License-Identifier: AGPL-3.0-or-later

const ID = "([A-Za-z0-9_-]{11})(?![A-Za-z0-9_-])";

const YOUTUBE_PATTERNS = [
    new RegExp(`(?:https?://)?(?:www\\.|m\\.|music\\.)?youtube\\.com/watch\\?(?:[^\\s]*&)?v=${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.)?youtu\\.be/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/shorts/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/embed/${ID}`, "i"),
    new RegExp(`(?:https?://)?(?:www\\.|m\\.)?youtube\\.com/live/${ID}`, "i"),
];

export function extractYoutubeId(text) {
    if (!text || typeof text !== "string") return null;
    for (const pattern of YOUTUBE_PATTERNS) {
        const match = text.match(pattern);
        if (match && match[1]) return match[1];
    }
    return null;
}

export function youtubeEmbedUrl(videoId) {
    return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(videoId)}?autoplay=1&rel=0`;
}

export function youtubeWatchUrl(videoId) {
    return `https://www.youtube.com/watch?v=${encodeURIComponent(videoId)}`;
}
