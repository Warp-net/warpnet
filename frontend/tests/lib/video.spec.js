import {describe, it, expect} from 'vitest';
import {
    isAcceptedVideo,
    validateVideoFile,
    acceptedVideoAccept,
    MAX_VIDEO_BYTES,
    normalizeVideoDataUrl,
    mimeForFile,
} from '@/lib/video';

const file = (type, name, size = 1024) => ({type, name, size});

describe('isAcceptedVideo', () => {
    it('accepts the ISO base media types the node can stamp', () => {
        expect(isAcceptedVideo(file('video/mp4', 'clip.mp4'))).toBe(true);
        expect(isAcceptedVideo(file('video/quicktime', 'clip.mov'))).toBe(true);
        expect(isAcceptedVideo(file('video/x-m4v', 'clip.m4v'))).toBe(true);
    });

    it('is case-insensitive about the MIME type', () => {
        expect(isAcceptedVideo(file('VIDEO/MP4', 'clip.mp4'))).toBe(true);
    });

    it('rejects containers the node cannot stamp ownership metadata into', () => {
        expect(isAcceptedVideo(file('video/webm', 'clip.webm'))).toBe(false);
        expect(isAcceptedVideo(file('video/x-msvideo', 'clip.avi'))).toBe(false);
        expect(isAcceptedVideo(file('image/png', 'not-a-video.png'))).toBe(false);
    });

    // Some platforms report an empty MIME type for a picked file.
    it('falls back to the extension when the MIME type is missing', () => {
        expect(isAcceptedVideo(file('', 'holiday.MP4'))).toBe(true);
        expect(isAcceptedVideo(file('', 'holiday.mov'))).toBe(true);
        expect(isAcceptedVideo(file('', 'holiday.webm'))).toBe(false);
        expect(isAcceptedVideo(file('', 'holiday'))).toBe(false);
    });

    it('rejects a missing file', () => {
        expect(isAcceptedVideo(null)).toBe(false);
        expect(isAcceptedVideo(undefined)).toBe(false);
    });
});

describe('validateVideoFile', () => {
    it('passes a small MP4', () => {
        expect(validateVideoFile(file('video/mp4', 'clip.mp4', 1024))).toBeNull();
    });

    it('names the accepted formats when rejecting an unsupported one', () => {
        const msg = validateVideoFile(file('video/webm', 'clip.webm'));
        expect(msg).toContain('MP4');
        expect(msg).toContain('MOV');
    });

    it('reports the actual size when the file is too big', () => {
        const msg = validateVideoFile(file('video/mp4', 'big.mp4', MAX_VIDEO_BYTES + 1));
        expect(msg).toContain('36 MB');
    });

    it('allows a file exactly at the limit', () => {
        expect(validateVideoFile(file('video/mp4', 'edge.mp4', MAX_VIDEO_BYTES))).toBeNull();
    });

    // Format is checked before size, so a huge WebM reports the real problem.
    it('reports the format problem before the size problem', () => {
        const msg = validateVideoFile(file('video/webm', 'big.webm', MAX_VIDEO_BYTES * 2));
        expect(msg).toContain('Unsupported');
    });
});

describe('acceptedVideoAccept', () => {
    it('lists both MIME types and extensions for the file picker', () => {
        expect(acceptedVideoAccept).toContain('video/mp4');
        expect(acceptedVideoAccept).toContain('.mov');
    });
});

describe('normalizeVideoDataUrl', () => {
    // The node's allow-list only knows video MIME types; a browser that
    // reported none would otherwise get the upload rejected.
    it('rewrites an octet-stream header using the extension', () => {
        const out = normalizeVideoDataUrl(
            'data:application/octet-stream;base64,AAAA',
            file('', 'holiday.mov'),
        );
        expect(out).toBe('data:video/quicktime;base64,AAAA');
    });

    it('keeps an already-accepted header', () => {
        const out = normalizeVideoDataUrl(
            'data:video/mp4;base64,AAAA',
            file('video/mp4', 'clip.mp4'),
        );
        expect(out).toBe('data:video/mp4;base64,AAAA');
    });

    it('preserves the payload byte for byte', () => {
        const payload = 'AAAABBBBCCCC==';
        const out = normalizeVideoDataUrl(
            `data:application/octet-stream;base64,${payload}`,
            file('', 'clip.mp4'),
        );
        expect(out.split(',')[1]).toBe(payload);
    });

    it('leaves the value alone when the file is not a known video', () => {
        const input = 'data:application/octet-stream;base64,AAAA';
        expect(normalizeVideoDataUrl(input, file('', 'notes.txt'))).toBe(input);
    });
});

describe('mimeForFile', () => {
    it('prefers the declared type when it is accepted', () => {
        expect(mimeForFile(file('video/mp4', 'clip.mov'))).toBe('video/mp4');
    });

    it('derives the type from the extension otherwise', () => {
        expect(mimeForFile(file('', 'clip.mov'))).toBe('video/quicktime');
        expect(mimeForFile(file('application/octet-stream', 'clip.m4v'))).toBe('video/x-m4v');
        expect(mimeForFile(file('', 'clip.webm'))).toBeNull();
    });
});
