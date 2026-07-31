import {describe, it, expect} from 'vitest';
import {extractYoutubeId, youtubeEmbedUrl, youtubeWatchUrl} from '@/lib/youtube';

describe('extractYoutubeId', () => {
    it('reads the id from a standard watch URL', () => {
        expect(extractYoutubeId('look at this https://www.youtube.com/watch?v=dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
    });

    it('reads the id from a short youtu.be URL', () => {
        expect(extractYoutubeId('https://youtu.be/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
    });

    it('reads the id from shorts, embed and live URLs', () => {
        expect(extractYoutubeId('https://www.youtube.com/shorts/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
        expect(extractYoutubeId('https://youtube.com/embed/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
        expect(extractYoutubeId('https://www.youtube.com/live/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
    });

    it('handles mobile and music subdomains', () => {
        expect(extractYoutubeId('https://m.youtube.com/watch?v=dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
        expect(extractYoutubeId('https://music.youtube.com/watch?v=dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
    });

    it('finds v= even when other query params come first', () => {
        expect(extractYoutubeId('https://www.youtube.com/watch?list=PL123&v=dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ');
    });

    it('returns null when there is no youtube link', () => {
        expect(extractYoutubeId('just some text')).toBeNull();
        expect(extractYoutubeId('https://vimeo.com/12345')).toBeNull();
        expect(extractYoutubeId('')).toBeNull();
        expect(extractYoutubeId(null)).toBeNull();
        expect(extractYoutubeId(undefined)).toBeNull();
    });

    // An id is exactly 11 chars; a longer token must not be truncated into a
    // bogus id, which would embed the wrong video.
    it('rejects an over-long id rather than truncating it', () => {
        expect(extractYoutubeId('https://youtu.be/dQw4w9WgXcQEXTRA')).toBeNull();
    });

    it('returns the first match when several links are present', () => {
        const text = 'https://youtu.be/aaaaaaaaaaa and https://youtu.be/bbbbbbbbbbb';
        expect(extractYoutubeId(text)).toBe('aaaaaaaaaaa');
    });
});

describe('youtube URLs', () => {
    // Playback must go through the nocookie host, and the id must be encoded
    // so a crafted value cannot break out of the URL.
    it('builds a nocookie embed URL', () => {
        const url = youtubeEmbedUrl('dQw4w9WgXcQ');
        expect(url).toContain('youtube-nocookie.com/embed/dQw4w9WgXcQ');
        expect(url).toContain('autoplay=1');
    });

    it('encodes the id in both URL builders', () => {
        expect(youtubeEmbedUrl('a/b?c=d')).toContain('a%2Fb%3Fc%3Dd');
        expect(youtubeWatchUrl('a/b?c=d')).toContain('a%2Fb%3Fc%3Dd');
    });
});
