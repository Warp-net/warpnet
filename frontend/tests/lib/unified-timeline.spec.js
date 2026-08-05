import { describe, it, expect, vi } from 'vitest';
import { createTimelineMerger } from '@/lib/unified-timeline';

const T0 = Date.parse('2026-08-01T00:00:00Z');
const at = (minutesAgo) => new Date(T0 - minutesAgo * 60000).toISOString();
const tweet = (id, minutesAgo, extra = {}) => ({
  id,
  created_at: at(minutesAgo),
  text: id,
  ...extra,
});

// A scripted source: each call pops the next page; the last page carries
// cursor 'end'. Records the cursors it was called with.
function scriptedSource(id, pages, opts = {}) {
  let call = 0;
  const cursors = [];
  return {
    id,
    ...opts,
    cursors,
    fetchPage: vi.fn(async (cursor) => {
      cursors.push(cursor);
      const page = pages[Math.min(call, pages.length - 1)];
      call++;
      return page;
    }),
  };
}

describe('createTimelineMerger', () => {
  it('interleaves two sources by created_at across page boundaries', async () => {
    const a = scriptedSource('a', [
      { tweets: [tweet('a1', 1), tweet('a2', 5)], cursor: 'ca' },
      { tweets: [tweet('a3', 9)], cursor: 'end' },
    ]);
    const b = scriptedSource('b', [
      { tweets: [tweet('b1', 3), tweet('b2', 7)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [a, b], pageSize: 10 });

    const { tweets, done } = await merger.nextPage();
    expect(tweets.map((t) => t.id)).toEqual(['a1', 'b1', 'a2', 'b2', 'a3']);
    expect(done).toBe(true);
    expect(a.cursors).toEqual(['', 'ca']);
  });

  it('respects pageSize and finishes only when everything is drained', async () => {
    const a = scriptedSource('a', [
      { tweets: [tweet('a1', 1), tweet('a2', 2), tweet('a3', 3)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [a], pageSize: 2 });

    const first = await merger.nextPage();
    expect(first.tweets.map((t) => t.id)).toEqual(['a1', 'a2']);
    expect(first.done).toBe(false);

    const second = await merger.nextPage();
    expect(second.tweets.map((t) => t.id)).toEqual(['a3']);
    expect(second.done).toBe(true);
  });

  it('preserves fetch order inside a source even when timestamps disagree', async () => {
    // a2 is newer than a1 but arrived second — the merger must not re-sort
    // within a source (Badger order is authoritative there).
    const a = scriptedSource('a', [
      { tweets: [tweet('a1', 5), tweet('a2', 1)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [a], pageSize: 10 });
    const { tweets } = await merger.nextPage();
    expect(tweets.map((t) => t.id)).toEqual(['a1', 'a2']);
  });

  it('does not let a rejecting source block the others, and retries it next page', async () => {
    let calls = 0;
    const bad = {
      id: 'bad',
      fetchPage: vi.fn(async () => {
        calls++;
        if (calls === 1) throw new Error('gateway down');
        return { tweets: [tweet('bad1', 4)], cursor: 'end' };
      }),
    };
    const good = scriptedSource('good', [
      { tweets: [tweet('g1', 2)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [bad, good], pageSize: 10 });

    const first = await merger.nextPage();
    expect(first.tweets.map((t) => t.id)).toEqual(['g1']);
    expect(first.done).toBe(false);

    const second = await merger.nextPage();
    expect(second.tweets.map((t) => t.id)).toEqual(['bad1']);
    expect(second.done).toBe(true);
  });

  it('writes off a source after two consecutive failing cycles', async () => {
    const bad = {
      id: 'bad',
      fetchPage: vi.fn(async () => {
        throw new Error('gateway down');
      }),
    };
    const merger = createTimelineMerger({ sources: [bad], pageSize: 10 });

    // one failure is still retryable — the feed must not end yet
    const first = await merger.nextPage();
    expect(first.tweets).toEqual([]);
    expect(first.done).toBe(false);

    // a second failing cycle makes it terminal
    const second = await merger.nextPage();
    expect(second.tweets).toEqual([]);
    expect(second.done).toBe(true);
    expect(bad.fetchPage).toHaveBeenCalledTimes(2);
  });

  it('excludes a source that misses the prime budget and appends it next page without a double fetch', async () => {
    vi.useFakeTimers();
    try {
      let resolveSlow;
      const slow = {
        id: 'slow',
        fetchPage: vi.fn(
          () => new Promise((resolve) => {
            resolveSlow = () => resolve({ tweets: [tweet('s1', 0)], cursor: 'end' });
          }),
        ),
      };
      const fast = scriptedSource('fast', [
        { tweets: [tweet('f1', 2)], cursor: 'end' },
      ]);
      const merger = createTimelineMerger({
        sources: [slow, fast],
        pageSize: 10,
        primeTimeoutMs: 1000,
      });

      const pending = merger.nextPage();
      await vi.advanceTimersByTimeAsync(1100);
      const first = await pending;
      expect(first.tweets.map((t) => t.id)).toEqual(['f1']);
      expect(first.done).toBe(false);

      // The in-flight fetch lands after the page closed…
      resolveSlow();
      await vi.advanceTimersByTimeAsync(0);

      // …and the next page serves it from the buffer without refetching.
      const second = await merger.nextPage();
      expect(second.tweets.map((t) => t.id)).toEqual(['s1']);
      expect(second.done).toBe(true);
      expect(slow.fetchPage).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('emits a tweet seen from two sources only once', async () => {
    const shared = tweet('https://mastodon.social/status/1', 3);
    const a = scriptedSource('a', [{ tweets: [shared], cursor: 'end' }]);
    const b = scriptedSource('b', [{ tweets: [{ ...shared }], cursor: 'end' }]);
    const merger = createTimelineMerger({ sources: [a, b], pageSize: 10 });
    const { tweets } = await merger.nextPage();
    expect(tweets).toHaveLength(1);
  });

  it('applies filters once per cycle and never emits filtered items', async () => {
    const applyFilters = vi.fn(async (tweets) => tweets.filter((t) => t.id !== 'spam'));
    const a = scriptedSource('a', [
      { tweets: [tweet('a1', 1), tweet('spam', 2), tweet('a2', 3)], cursor: 'end' },
    ]);
    const b = scriptedSource('b', [
      { tweets: [tweet('b1', 4)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [a, b], pageSize: 10, applyFilters });

    const { tweets } = await merger.nextPage();
    expect(tweets.map((t) => t.id)).toEqual(['a1', 'a2', 'b1']);
    expect(applyFilters).toHaveBeenCalledTimes(1);
  });

  it('bypasses filters for a prefiltered source', async () => {
    const applyFilters = vi.fn(async () => []);
    const a = scriptedSource(
      'a',
      [{ tweets: [tweet('a1', 1)], cursor: 'end' }],
      { prefiltered: true },
    );
    const merger = createTimelineMerger({ sources: [a], pageSize: 10, applyFilters });
    const { tweets } = await merger.nextPage();
    expect(tweets.map((t) => t.id)).toEqual(['a1']);
    expect(applyFilters).not.toHaveBeenCalled();
  });

  it('passes items through when the filter itself fails', async () => {
    const applyFilters = vi.fn(async () => {
      throw new Error('filters unavailable');
    });
    const a = scriptedSource('a', [{ tweets: [tweet('a1', 1)], cursor: 'end' }]);
    const merger = createTimelineMerger({ sources: [a], pageSize: 10, applyFilters });
    const { tweets } = await merger.nextPage();
    expect(tweets.map((t) => t.id)).toEqual(['a1']);
  });

  describe('refreshNewest', () => {
    it('returns only unseen items newer than the source max, sorted desc', async () => {
      const pages = [
        { tweets: [tweet('m1', 10), tweet('m2', 20)], cursor: 'end' },
        // refresh page: two new posts plus the already-known head
        { tweets: [tweet('m4', 1), tweet('m3', 5), tweet('m1', 10)], cursor: 'end' },
      ];
      const m = scriptedSource('m', pages);
      const merger = createTimelineMerger({ sources: [m], pageSize: 10 });
      await merger.nextPage();

      const fresh = await merger.refreshNewest();
      expect(fresh.map((t) => t.id)).toEqual(['m4', 'm3']);
      // the refresh used a blank cursor, not the paging cursor
      expect(m.cursors).toEqual(['', '']);

      // a second refresh with the same page yields nothing new
      const again = await merger.refreshNewest();
      expect(again).toEqual([]);
    });

    it('skips sources marked skipRefresh and sources that never yielded', async () => {
      const w = scriptedSource(
        'warpnet',
        [{ tweets: [tweet('w1', 1)], cursor: 'end' }],
        { skipRefresh: true, prefiltered: true },
      );
      const cold = {
        id: 'cold',
        fetchPage: vi.fn(async () => {
          throw new Error('still cold');
        }),
      };
      const merger = createTimelineMerger({ sources: [w, cold], pageSize: 10 });
      await merger.nextPage();
      w.fetchPage.mockClear();
      cold.fetchPage.mockClear();

      const fresh = await merger.refreshNewest();
      expect(fresh).toEqual([]);
      expect(w.fetchPage).not.toHaveBeenCalled();
      expect(cold.fetchPage).not.toHaveBeenCalled();
    });
  });

  it('reset() restores a fresh first page', async () => {
    const a = scriptedSource('a', [
      { tweets: [tweet('a1', 1)], cursor: 'end' },
      { tweets: [tweet('a1', 1)], cursor: 'end' },
    ]);
    const merger = createTimelineMerger({ sources: [a], pageSize: 10 });

    const first = await merger.nextPage();
    expect(first.done).toBe(true);

    merger.reset();
    expect(merger.done).toBe(false);

    const again = await merger.nextPage();
    expect(again.tweets.map((t) => t.id)).toEqual(['a1']);
    expect(a.cursors).toEqual(['', '']);
  });
});
