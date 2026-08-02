import {describe, it, expect, beforeEach} from 'vitest';
import {
    EMOJI_CATEGORIES,
    SKIN_TONES,
    applyTone,
    clampRunes,
    findEmoji,
    insertEmoji,
    loadRecent,
    loadTone,
    pushRecent,
    runeLength,
    saveTone,
    searchEmojis,
} from '@/lib/emoji';

// A textarea stand-in: only the caret API matters to insertEmoji.
const field = (start, end = start) => ({selectionStart: start, selectionEnd: end});

describe('runeLength', () => {
    it('counts code points, matching Go utf8.RuneCountInString', () => {
        expect(runeLength('abc')).toBe(3);
        // These cost 2 each under String.length, which is what made the old
        // counters disagree with the node.
        expect(runeLength('😀')).toBe(1);
        expect(runeLength('😀😀😀')).toBe(3);
        expect('😀😀😀'.length).toBe(6);
    });

    it('counts a variation selector as its own rune, like the node does', () => {
        expect(runeLength('❤️')).toBe(2);
    });

    it('treats empty input as zero', () => {
        expect(runeLength('')).toBe(0);
        expect(runeLength(undefined)).toBe(0);
        expect(runeLength(null)).toBe(0);
    });
});

describe('clampRunes', () => {
    it('leaves text within the budget untouched', () => {
        expect(clampRunes('hello', 10)).toBe('hello');
        expect(clampRunes('😀😀', 2)).toBe('😀😀');
    });

    it('trims to the rune budget', () => {
        expect(clampRunes('abcdef', 3)).toBe('abc');
        expect(clampRunes('😀😀😀', 2)).toBe('😀😀');
    });

    it('never splits a surrogate pair', () => {
        const clamped = clampRunes('😀😀', 1);
        expect(clamped).toBe('😀');
        // A naive slice(0, 1) would leave half a pair and marshal to U+FFFD.
        expect(clamped).not.toContain('�');
        expect([...clamped].length).toBe(1);
    });

    it('handles empty input', () => {
        expect(clampRunes('', 5)).toBe('');
        expect(clampRunes(undefined, 5)).toBe('');
    });
});

describe('insertEmoji', () => {
    it('inserts at the caret and reports where the caret lands', () => {
        const result = insertEmoji({text: 'ab', emoji: '😀', field: field(1), limit: 280});
        expect(result.text).toBe('a😀b');
        // The caret is a UTF-16 offset, so it advances by the emoji's length.
        expect(result.caret).toBe(1 + '😀'.length);
    });

    it('replaces the current selection', () => {
        const result = insertEmoji({text: 'abcd', emoji: '😀', field: field(1, 3), limit: 280});
        expect(result.text).toBe('a😀d');
    });

    it('appends when the field has no caret information', () => {
        expect(insertEmoji({text: 'ab', emoji: '😀', field: null, limit: 280}).text).toBe('ab😀');
    });

    it('refuses an insert that would exceed the rune budget', () => {
        expect(insertEmoji({text: 'ab', emoji: '😀', field: field(2), limit: 3})).toEqual({
            text: 'ab😀',
            caret: 2 + '😀'.length,
        });
        expect(insertEmoji({text: 'abc', emoji: '😀', field: field(3), limit: 3})).toBeNull();
    });

    it('measures the budget in runes, not UTF-16 units', () => {
        // Three emoji are 6 UTF-16 units but only 3 runes, so a limit of 4
        // still has room for one more.
        const result = insertEmoji({text: '😀😀😀', emoji: '😀', field: field(6), limit: 4});
        expect(result).not.toBeNull();
        expect(runeLength(result.text)).toBe(4);
    });
});

describe('applyTone', () => {
    const light = SKIN_TONES[1].modifier;

    it('appends the modifier to emoji that accept one', () => {
        expect(applyTone('👍', light, 1)).toBe('👍' + light);
    });

    it('leaves emoji that take no tone alone', () => {
        expect(applyTone('🍕', light, undefined)).toBe('🍕');
    });

    it('is a no-op for the default tone', () => {
        expect(applyTone('👍', '', 1)).toBe('👍');
    });

    it('drops the variation selector so the modifier composes', () => {
        // U+270C U+FE0F U+1F3FB renders as two glyphs; U+270C U+1F3FB is one.
        expect(applyTone('✌️', light, 1)).toBe('✌' + light);
    });
});

describe('searchEmojis', () => {
    it('finds emoji by their Unicode name', () => {
        expect(searchEmojis('pizza').map((e) => e[0])).toContain('🍕');
    });

    it('finds emoji by colloquial alias', () => {
        expect(searchEmojis('lol').map((e) => e[0])).toContain('😂');
        expect(searchEmojis('thumbs up').map((e) => e[0])).toContain('👍');
    });

    it('requires every term to match', () => {
        expect(searchEmojis('pizza rocket')).toEqual([]);
    });

    it('returns nothing for an empty query', () => {
        expect(searchEmojis('')).toEqual([]);
        expect(searchEmojis('   ')).toEqual([]);
    });
});

describe('emoji catalogue', () => {
    it('has no duplicate characters across categories', () => {
        const all = EMOJI_CATEGORIES.flatMap((c) => c.emojis.map((e) => e[0]));
        expect(all.length).toBe(new Set(all).size);
    });

    it('gives every entry a character and search keywords', () => {
        for (const category of EMOJI_CATEGORIES) {
            expect(category.emojis.length).toBeGreaterThan(0);
            for (const [char, keywords] of category.emojis) {
                expect(char.length).toBeGreaterThan(0);
                expect(keywords.trim().length).toBeGreaterThan(0);
            }
        }
    });

    it('looks an entry up by character', () => {
        expect(findEmoji('🍕')[1]).toContain('pizza');
        expect(findEmoji('not-an-emoji')).toBeUndefined();
    });
});

describe('recents and tone persistence', () => {
    beforeEach(() => {
        window.localStorage.clear();
    });

    it('starts empty', () => {
        expect(loadRecent()).toEqual([]);
        expect(loadTone()).toBe(0);
    });

    it('keeps the most recent pick first without duplicating it', () => {
        pushRecent(['😀', 'grinning face']);
        pushRecent(['🍕', 'pizza']);
        pushRecent(['😀', 'grinning face']);
        expect(loadRecent().map((e) => e[0])).toEqual(['😀', '🍕']);
    });

    it('caps the recents list', () => {
        for (let i = 0; i < 40; i++) pushRecent([`e${i}`, 'x']);
        expect(loadRecent().length).toBe(32);
    });

    it('ignores a corrupted or legacy recents payload', () => {
        window.localStorage.setItem('warpnet.emoji.recent', '{"not":"an array"}');
        expect(loadRecent()).toEqual([]);
        // The first shipped format stored bare strings rather than pairs.
        window.localStorage.setItem('warpnet.emoji.recent', '["😀"]');
        expect(loadRecent()).toEqual([]);
    });

    it('round-trips the chosen skin tone', () => {
        saveTone(3);
        expect(loadTone()).toBe(3);
    });

    it('falls back to the default tone when the stored one is unknown', () => {
        saveTone(99);
        expect(loadTone()).toBe(0);
    });
});
