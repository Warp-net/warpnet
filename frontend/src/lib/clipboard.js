// SPDX-License-Identifier: AGPL-3.0-or-later

const isImage = (type) => !!type && type.startsWith("image/");

// WebKitGTK, which the Linux desktop app runs on, leaves a copied image out of
// the paste event and hands it only to the async Clipboard API, so a paste
// carrying neither files nor plain text falls back to reading that.
export async function pastedImages(event) {
    const data = event.clipboardData;
    const files = Array.from(data?.files || []).filter(f => isImage(f.type));
    if (files.length > 0) {
        event.preventDefault();
        return files;
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
