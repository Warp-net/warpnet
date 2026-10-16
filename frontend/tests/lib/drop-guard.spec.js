import {describe, it, expect, beforeAll, afterEach} from 'vitest';
import {createEvent, fireEvent} from '@testing-library/vue';
import {blockStrayDrops} from '@/lib/drop-guard';

const drag = (type, target, types) => {
    const event = createEvent[type](target, {dataTransfer: {types, dropEffect: 'copy', files: []}});
    fireEvent(target, event);
    return event;
};

const place = (html) => {
    document.body.innerHTML = html;
    return document.body.firstElementChild;
};

beforeAll(() => blockStrayDrops(window));

afterEach(() => {
    document.body.innerHTML = '';
});

describe('blockStrayDrops', () => {
    it('keeps a file dropped where nothing handles it from replacing the app', () => {
        const timeline = place('<div><span>tweet</span></div>').firstElementChild;

        for (const type of ['dragEnter', 'dragOver', 'drop']) {
            expect(drag(type, timeline, ['Files']).defaultPrevented).toBe(true);
        }
        expect(drag('dragOver', timeline, ['Files']).dataTransfer.dropEffect).toBe('none');
    });

    it('blocks a bare uri-list, which is how WebKitGTK shows both a dragged file and a link', () => {
        const side = place('<nav></nav>');

        expect(drag('dragEnter', side, ['text/uri-list', 'text/html']).defaultPrevented).toBe(true);
        expect(drag('drop', side, ['text/plain', 'text/html', 'text/uri-list']).defaultPrevented).toBe(true);
    });

    it('leaves a plain text drag to the field under it', () => {
        const field = place('<textarea></textarea>');

        for (const type of ['dragEnter', 'dragOver', 'drop']) {
            const event = drag(type, field, ['text/plain', 'text/html']);
            expect(event.defaultPrevented).toBe(false);
            expect(event.dataTransfer.dropEffect).toBe('copy');
        }
    });

    it('leaves a drag to a drop zone that has already cancelled it', () => {
        const zone = place('<div><textarea></textarea></div>');
        let dropped = 0;
        zone.addEventListener('dragover', e => e.preventDefault());
        zone.addEventListener('drop', e => { e.preventDefault(); dropped++; });

        expect(drag('dragOver', zone.firstElementChild, ['Files']).dataTransfer.dropEffect).toBe('copy');
        drag('drop', zone.firstElementChild, ['Files']);
        expect(dropped).toBe(1);
    });

    it('leaves a native file input its own drop, as the composer overlay needs on WebKitGTK', () => {
        const input = place('<input type="file" multiple>');

        for (const type of ['dragEnter', 'dragOver', 'drop']) {
            expect(drag(type, input, ['text/uri-list']).defaultPrevented).toBe(false);
        }
    });

    it('ignores a drag event without a dataTransfer', () => {
        const box = place('<div></div>');
        const event = new Event('dragover', {bubbles: true, cancelable: true});

        box.dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
    });
});
