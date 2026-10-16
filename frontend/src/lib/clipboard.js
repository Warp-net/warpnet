// SPDX-License-Identifier: AGPL-3.0-or-later

import {toast} from "@/lib/toast";

const isImage = (type) => !!type && type.startsWith("image/");
const isMedia = (type) => isImage(type) || (!!type && type.startsWith("video/"));

// pastedFiles returns the images and videos a paste carries. WebKitGTK, which
// the Linux desktop app runs on, leaves a copied image out of the paste event
// and hands it only to the async Clipboard API, so a paste carrying neither
// files nor plain text falls back to reading that.
export async function pastedFiles(event) {
    const data = event.clipboardData;
    const files = Array.from(data?.files || []).filter(f => isMedia(f.type));
    if (files.length > 0) {
        event.preventDefault();
        return files;
    }
    // Files copied in a file manager reach WebKitGTK only as a text/uri-list
    // that reads back empty (a copied link reads back as itself): the page
    // can't read them, and the paste would insert their paths.
    if (data?.files?.length === 0 && data.types?.includes("text/uri-list") && !data.getData("text/uri-list")) {
        event.preventDefault();
        toast.info("Copied files can't be pasted here. Use the attach button instead.");
        return [];
    }
    if (data?.types?.includes("text/plain") || !navigator.clipboard?.read) {
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
