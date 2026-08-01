import {describe, it, expect, vi} from 'vitest';
import {
    isAcceptedVideo,
    validateVideoFile,
    acceptedVideoAccept,
    MAX_VIDEO_BYTES,
    POSTER_MAX_WIDTH,
    POSTER_SEEK_SECONDS,
    normalizeVideoDataUrl,
    mimeForFile,
    captureVideoPoster,
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

describe('captureVideoPoster', () => {
    const fakeEnv = (overrides = {}) => {
        const video = {};
        const canvas = {
            getContext: () => ({drawImage: () => {}}),
            toDataURL: () => 'data:image/jpeg;base64,POSTER',
        };
        return {
            video,
            canvas,
            env: {
                createElement: tag => (tag === 'video' ? video : canvas),
                createObjectURL: () => 'blob:clip',
                revokeObjectURL: vi.fn(),
                ...overrides,
            },
        };
    };

    const decode = (video, {duration = 10, width = 1280, height = 720} = {}) => {
        video.duration = duration;
        video.videoWidth = width;
        video.videoHeight = height;
        video.onloadeddata();
    };

    it('returns a JPEG data URL for a decodable clip', async () => {
        const {video, env} = fakeEnv();
        const pending = captureVideoPoster(file('video/mp4', 'clip.mp4'), env);

        decode(video);
        video.onseeked();

        await expect(pending).resolves.toBe('data:image/jpeg;base64,POSTER');
        expect(env.revokeObjectURL).toHaveBeenCalledWith('blob:clip');
    });

    it('takes the frame a second in, past any fade-in', async () => {
        const {video, env} = fakeEnv();
        const pending = captureVideoPoster(file('video/mp4', 'clip.mp4'), env);

        decode(video);
        expect(video.currentTime).toBe(POSTER_SEEK_SECONDS);

        video.onseeked();
        await pending;
    });

    it('falls back to the midpoint of a clip shorter than that', async () => {
        const {video, env} = fakeEnv();
        const pending = captureVideoPoster(file('video/mp4', 'blink.mp4'), env);

        decode(video, {duration: 0.8});
        expect(video.currentTime).toBe(0.4);

        video.onseeked();
        await pending;
    });

    it('downscales a large frame to the poster width', async () => {
        const {video, canvas, env} = fakeEnv();
        const pending = captureVideoPoster(file('video/mp4', 'uhd.mp4'), env);

        decode(video, {width: 1920, height: 1080});
        video.onseeked();
        await pending;

        expect(canvas.width).toBe(POSTER_MAX_WIDTH);
        expect(canvas.height).toBe(360);
    });

    it('resolves null when the browser cannot decode the file', async () => {
        const {video, env} = fakeEnv();
        const pending = captureVideoPoster(file('video/quicktime', 'hevc.mov'), env);

        video.onerror();

        await expect(pending).resolves.toBeNull();
        expect(env.revokeObjectURL).toHaveBeenCalledWith('blob:clip');
    });

    it('resolves null when the decoder never reports back', async () => {
        const {env} = fakeEnv();
        await expect(
            captureVideoPoster(file('video/mp4', 'stuck.mp4'), {...env, timeoutMs: 0}),
        ).resolves.toBeNull();
    });

    it('resolves null without a file', async () => {
        await expect(captureVideoPoster(null)).resolves.toBeNull();
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
