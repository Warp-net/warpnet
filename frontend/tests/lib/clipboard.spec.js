import {describe, it, expect, vi, afterEach} from 'vitest';
import {pastedFiles} from '@/lib/clipboard';
import {ClipboardFiles} from '@/lib/transport';

vi.mock('@/lib/transport', () => ({ClipboardFiles: vi.fn()}));

const pasteEvent = (clipboardData) => ({clipboardData, preventDefault: vi.fn()});

const stubClipboard = (read) => {
    Object.defineProperty(navigator, 'clipboard', {value: {read}, configurable: true});
    return read;
};

afterEach(() => {
    delete navigator.clipboard;
    ClipboardFiles.mockReset();
});

describe('pastedFiles', () => {
    it('takes images and videos straight from the paste event and keeps them out of the text', async () => {
        const png = new File(['x'], 'shot.png', {type: 'image/png'});
        const mp4 = new File(['x'], 'clip.mp4', {type: 'video/mp4'});
        const txt = new File(['x'], 'notes.txt', {type: 'text/plain'});
        const event = pasteEvent({files: [png, mp4, txt], types: ['Files']});

        expect(await pastedFiles(event)).toEqual([png, mp4]);
        expect(event.preventDefault).toHaveBeenCalled();
    });

    it('leaves a text paste to the textarea', async () => {
        const read = stubClipboard(vi.fn());
        const event = pasteEvent({files: [], types: ['text/plain']});

        expect(await pastedFiles(event)).toEqual([]);
        expect(event.preventDefault).not.toHaveBeenCalled();
        expect(read).not.toHaveBeenCalled();
    });

    it('asks the desktop shell for files WebKitGTK hides behind a bare uri list', async () => {
        ClipboardFiles.mockResolvedValue([{name: 'clip.mp4', type: 'video/mp4', data: btoa('movie')}]);
        const event = pasteEvent({files: [], types: ['text/uri-list'], getData: () => ''});

        const [file] = await pastedFiles(event);

        expect(event.preventDefault).toHaveBeenCalled();
        expect(file.name).toBe('clip.mp4');
        expect(file.type).toBe('video/mp4');
        expect(file.size).toBe(5);
    });

    it('leaves a copied link to the textarea', async () => {
        const event = pasteEvent({files: [], types: ['text/uri-list'], getData: () => 'https://example.com\r\n'});

        expect(await pastedFiles(event)).toEqual([]);
        expect(event.preventDefault).not.toHaveBeenCalled();
        expect(ClipboardFiles).not.toHaveBeenCalled();
    });

    it('reads the image WebKitGTK leaves out of the paste event', async () => {
        const blob = new Blob(['x'], {type: 'image/png'});
        stubClipboard(vi.fn().mockResolvedValue([
            {types: ['text/html'], getType: vi.fn()},
            {types: ['image/png'], getType: vi.fn().mockResolvedValue(blob)},
        ]));

        expect(await pastedFiles(pasteEvent({files: [], types: []}))).toEqual([blob]);
    });

    it('gives up quietly when the clipboard read is refused', async () => {
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
        stubClipboard(vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError')));

        expect(await pastedFiles(pasteEvent({files: [], types: []}))).toEqual([]);
        warn.mockRestore();
    });

    it('has nothing to read without the async Clipboard API', async () => {
        expect(await pastedFiles(pasteEvent({files: [], types: []}))).toEqual([]);
    });
});
