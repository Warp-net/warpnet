import {describe, it, expect, vi, afterEach} from 'vitest';
import {pastedImages} from '@/lib/clipboard';

const pasteEvent = (clipboardData) => ({clipboardData, preventDefault: vi.fn()});

const stubClipboard = (read) => {
    Object.defineProperty(navigator, 'clipboard', {value: {read}, configurable: true});
    return read;
};

afterEach(() => {
    delete navigator.clipboard;
});

describe('pastedImages', () => {
    it('takes images straight from the paste event and keeps them out of the text', async () => {
        const png = new File(['x'], 'shot.png', {type: 'image/png'});
        const txt = new File(['x'], 'notes.txt', {type: 'text/plain'});
        const event = pasteEvent({files: [png, txt], types: ['Files']});

        expect(await pastedImages(event)).toEqual([png]);
        expect(event.preventDefault).toHaveBeenCalled();
    });

    it('leaves a text paste to the textarea', async () => {
        const read = stubClipboard(vi.fn());
        const event = pasteEvent({files: [], types: ['text/plain']});

        expect(await pastedImages(event)).toEqual([]);
        expect(event.preventDefault).not.toHaveBeenCalled();
        expect(read).not.toHaveBeenCalled();
    });

    it('reads the image WebKitGTK leaves out of the paste event', async () => {
        const blob = new Blob(['x'], {type: 'image/png'});
        stubClipboard(vi.fn().mockResolvedValue([
            {types: ['text/html'], getType: vi.fn()},
            {types: ['image/png'], getType: vi.fn().mockResolvedValue(blob)},
        ]));

        expect(await pastedImages(pasteEvent({files: [], types: []}))).toEqual([blob]);
    });

    it('gives up quietly when the clipboard read is refused', async () => {
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
        stubClipboard(vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError')));

        expect(await pastedImages(pasteEvent({files: [], types: []}))).toEqual([]);
        warn.mockRestore();
    });

    it('has nothing to read without the async Clipboard API', async () => {
        expect(await pastedImages(pasteEvent({files: [], types: []}))).toEqual([]);
    });
});
