// SPDX-License-Identifier: AGPL-3.0-or-later
// Client-side gate for video attachments. The node re-checks everything here
// (see core/handler/video.go); this exists so an unusable file is refused
// instantly instead of after a multi-megabyte round trip.

// Mirrors handler.maxVideoSize on the node.
export const MAX_VIDEO_BYTES = 36 * 1024 * 1024;

// The node only accepts ISO base media files, because that is where it can
// stamp the ownership metadata (an MP4 `uuid` box), mirroring the EXIF stamp
// on images. Codecs are deliberately not inspected: decoding is the operating
// system's job, so an exotic codec inside a valid MP4 is accepted and simply
// won't play on a machine that lacks the codec.
export const ACCEPTED_VIDEO_MIME = ["video/mp4", "video/quicktime", "video/x-m4v"];
export const ACCEPTED_VIDEO_EXT = [".mp4", ".m4v", ".mov"];

export const acceptedVideoAccept = ACCEPTED_VIDEO_MIME.concat(ACCEPTED_VIDEO_EXT).join(",");

export function isAcceptedVideo(file) {
    if (!file) return false;
    if (file.type) {
        return ACCEPTED_VIDEO_MIME.includes(file.type.toLowerCase());
    }
    // Some platforms hand over an empty MIME type; fall back to the extension.
    const name = (file.name || "").toLowerCase();
    return ACCEPTED_VIDEO_EXT.some(ext => name.endsWith(ext));
}

const EXT_TO_MIME = {
    ".mp4": "video/mp4",
    ".m4v": "video/x-m4v",
    ".mov": "video/quicktime",
};

// mimeForFile resolves the MIME type the node should be told about, falling
// back to the extension when the platform reported none.
export function mimeForFile(file) {
    if (!file) return null;
    const declared = (file.type || "").toLowerCase();
    if (ACCEPTED_VIDEO_MIME.includes(declared)) {
        return declared;
    }
    const name = (file.name || "").toLowerCase();
    const ext = Object.keys(EXT_TO_MIME).find(e => name.endsWith(e));
    return ext ? EXT_TO_MIME[ext] : null;
}

// normalizeVideoDataUrl rewrites the data-URL header to an accepted video MIME
// type. FileReader emits "data:application/octet-stream;base64,..." when the
// platform reported no type, which the node's allow-list would reject even
// though the file itself is fine.
export function normalizeVideoDataUrl(dataUrl, file) {
    if (typeof dataUrl !== "string") return dataUrl;
    const mime = mimeForFile(file);
    if (!mime) return dataUrl;
    const comma = dataUrl.indexOf(",");
    if (comma < 0) return dataUrl;
    return `data:${mime};base64,${dataUrl.slice(comma + 1)}`;
}

// validateVideoFile returns null when the file may be uploaded, otherwise a
// message written for the person who picked the file.
export function validateVideoFile(file) {
    if (!file) {
        return "No video file selected.";
    }
    if (!isAcceptedVideo(file)) {
        return "Unsupported video format. Only MP4 and MOV videos can be attached.";
    }
    if (file.size > MAX_VIDEO_BYTES) {
        const mb = Math.round(file.size / (1024 * 1024));
        return `Video is too large (${mb} MB). The maximum size is 36 MB.`;
    }
    return null;
}
