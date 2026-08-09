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

export const POSTER_MAX_WIDTH = 640;
export const POSTER_SEEK_SECONDS = 1;
export const POSTER_TIMEOUT_MS = 10000;

// Grabs a frame near the start of a freshly picked video so the post can show a
// still instead of a blank placeholder. Best effort: resolves to null whenever
// the browser cannot decode the file (e.g. HEVC .mov outside Safari).
export function captureVideoPoster(file, opts = {}) {
    const {
        maxWidth = POSTER_MAX_WIDTH,
        seekSeconds = POSTER_SEEK_SECONDS,
        timeoutMs = POSTER_TIMEOUT_MS,
        createElement = tag => (typeof document === "undefined" ? null : document.createElement(tag)),
        createObjectURL = f => (typeof URL === "undefined" || !URL.createObjectURL ? null : URL.createObjectURL(f)),
        revokeObjectURL = url => {
            if (typeof URL !== "undefined" && URL.revokeObjectURL) URL.revokeObjectURL(url);
        },
    } = opts;

    return new Promise(resolve => {
        const video = file ? createElement("video") : null;
        const url = video ? createObjectURL(file) : null;
        if (!url) {
            resolve(null);
            return;
        }

        let settled = false;
        const finish = poster => {
            if (settled) return;
            settled = true;
            clearTimeout(timer);
            video.onloadeddata = null;
            video.onseeked = null;
            video.onerror = null;
            revokeObjectURL(url);
            resolve(poster || null);
        };
        const timer = setTimeout(() => finish(null), timeoutMs);

        video.onerror = () => finish(null);
        video.onloadeddata = () => {
            const duration = Number(video.duration);
            // The opening second is often a fade-in or a black lead-in, so take
            // the frame a second in, or the midpoint of a clip too short for that.
            const target = Number.isFinite(duration) && duration > 0
                ? Math.min(seekSeconds, duration / 2)
                : seekSeconds;
            if (target > 0) {
                video.currentTime = target;
                return;
            }
            finish(drawPoster(video, maxWidth, createElement));
        };
        video.onseeked = () => finish(drawPoster(video, maxWidth, createElement));
        video.muted = true;
        video.playsInline = true;
        video.preload = "metadata";
        video.src = url;
    });
}

function drawPoster(video, maxWidth, createElement) {
    const {videoWidth: width, videoHeight: height} = video;
    if (!width || !height) return null;

    const canvas = createElement("canvas");
    const ctx = canvas && canvas.getContext ? canvas.getContext("2d") : null;
    if (!ctx) return null;

    const scale = Math.min(1, maxWidth / width);
    canvas.width = Math.round(width * scale);
    canvas.height = Math.round(height * scale);
    try {
        ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
        return canvas.toDataURL("image/jpeg", 0.7);
    } catch (err) {
        return null;
    }
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
