// SPDX-License-Identifier: AGPL-3.0-or-later

import {ClipboardFiles} from "@/lib/transport";

const isImage = (type) => !!type && type.startsWith("image/");
const isMedia = (type) => isImage(type) || (!!type && type.startsWith("video/"));

// pastedFiles returns the images and videos a paste carries. WebKitGTK, which
// the Linux desktop app runs on, leaves both a copied image and files copied
// in a file manager out of the paste event, so it falls back to the async
// Clipboard API and to the desktop shell respectively.
export async function pastedFiles(event) {
    const data = event.clipboardData;
    const files = Array.from(data?.files || []).filter(f => isMedia(f.type));
    if (files.length > 0) {
        event.preventDefault();
        return files;
    }
    const types = Array.from(data?.types || []);
    // Copied files show up as a text/uri-list whose file:// URIs read back
    // empty (a copied link reads back as itself), and WebKitGTK would paste their paths.
    if (data?.files?.length === 0 && types.includes("text/uri-list") && !data.getData("text/uri-list")) {
        event.preventDefault();
        const copied = await ClipboardFiles();
        return (copied || []).map(toFile);
    }
    if (types.includes("text/plain") || !navigator.clipboard?.read) {
        return [];
    }
    let items;
    try {
        items = await navigator.clipboard.read();
    } catch (err) {
        console.warn("clipboard read refused:", err);
        return [];
    }
    const images = [];
    for (const item of items) {
        const type = item.types.find(isImage);
        if (type) images.push(await item.getType(type));
    }
    return images;
}

// The bound Go method hands file contents over as base64.
function toFile({name, type, data}) {
    const binary = atob(data || "");
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
    }
    return new File([bytes], name, {type});
}
