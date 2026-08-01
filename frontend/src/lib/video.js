// SPDX-License-Identifier: AGPL-3.0-or-later

export const MAX_VIDEO_BYTES = 36 * 1024 * 1024;

export const ACCEPTED_VIDEO_MIME = ["video/mp4", "video/quicktime", "video/x-m4v"];
export const ACCEPTED_VIDEO_EXT = [".mp4", ".m4v", ".mov"];

export const acceptedVideoAccept = ACCEPTED_VIDEO_MIME.concat(ACCEPTED_VIDEO_EXT).join(",");

export function isAcceptedVideo(file) {
    if (!file) return false;
    if (file.type) {
        return ACCEPTED_VIDEO_MIME.includes(file.type.toLowerCase());
    }
    const name = (file.name || "").toLowerCase();
    return ACCEPTED_VIDEO_EXT.some(ext => name.endsWith(ext));
}

const EXT_TO_MIME = {
    ".mp4": "video/mp4",
    ".m4v": "video/x-m4v",
    ".mov": "video/quicktime",
};

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

export function normalizeVideoDataUrl(dataUrl, file) {
    if (typeof dataUrl !== "string") return dataUrl;
    const mime = mimeForFile(file);
    if (!mime) return dataUrl;
    const comma = dataUrl.indexOf(",");
    if (comma < 0) return dataUrl;
    return `data:${mime};base64,${dataUrl.slice(comma + 1)}`;
}

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
