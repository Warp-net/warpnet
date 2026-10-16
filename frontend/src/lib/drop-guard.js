// SPDX-License-Identifier: AGPL-3.0-or-later

// A file or link dropped where no handler takes it is loaded in place of the
// app: by WebKitGTK in the desktop app, which leaves no way back, and by the
// browser tab alike. Listening on window runs after the page's own handlers,
// so a drag one of them has cancelled is theirs. Plain text keeps its default,
// and so does a native file input, which takes dropped files itself.
const carriesFileOrLink = (event) => {
    const types = Array.from(event.dataTransfer?.types || []);
    return types.includes("Files") || types.includes("text/uri-list");
};

function blockStrayDrop(event) {
    if (event.defaultPrevented || !carriesFileOrLink(event)) return;
    if (event.target instanceof HTMLInputElement && event.target.type === "file") return;
    event.preventDefault();
    // Refuse the drop outright, so the cursor shows it and the source hears it.
    if (event.type === "dragover") event.dataTransfer.dropEffect = "none";
}

// dragenter is all an element gets on the first move over it, and a drop
// right then would load the URL without a drop event.
export function blockStrayDrops(target) {
    for (const type of ["dragenter", "dragover", "drop"]) {
        target.addEventListener(type, blockStrayDrop);
    }
}
