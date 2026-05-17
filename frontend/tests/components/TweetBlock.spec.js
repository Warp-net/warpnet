import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor } from '@testing-library/vue';
import { nextTick } from 'vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    hasLiker: vi.fn(),
    hasRetweeter: vi.fn(),
    viewTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

let observerInstances = [];

class FakeIntersectionObserver {
  constructor(callback, options) {
    this.callback = callback;
    this.options = options;
    this.elements = [];
    observerInstances.push(this);
  }
  observe(el) {
    this.elements.push(el);
  }
  unobserve(el) {
    this.elements = this.elements.filter((e) => e !== el);
  }
  disconnect() {
    this.elements = [];
  }
  trigger(entry) {
    this.callback([entry], this);
  }
}

const renderTweet = (tweet) =>
  render(TweetBlock, {
    props: { tweet },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
      stubs: {
        ReplyOverlay: true,
      },
    },
  });

let logSpy, errSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  vi.clearAllMocks();
  observerInstances = [];

  warpnetService.getProfile.mockResolvedValue({
    id: 'author1',
    username: 'author',
    avatar_key: '',
  });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({
    user_id: 'viewer1',
    node_id: 'node-viewer',
  });
  warpnetService.getTweetStats.mockResolvedValue({
    tweet_id: 't1',
    tweets_count: 0,
    retweets_count: 0,
    likes_count: 0,
    replies_count: 0,
    views_count: 0,
  });
  warpnetService.hasLiker.mockResolvedValue(false);
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.viewTweet.mockResolvedValue(7);
});

const baseTweet = {
  id: 't1',
  user_id: 'author1',
  username: 'author',
  text: 'hello world',
  created_at: '2026-05-04T00:00:00Z',
  parent_id: '',
  root_id: '',
  retweeted_by: '',
  image_keys: [],
};

describe('TweetBlock view tracking', () => {
  it('records a view and updates the count when the tweet becomes visible', async () => {
    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.7,
      intersectionRect: { height: 200 },
      rootBounds: { height: 800 },
    });

    await waitFor(() => expect(warpnetService.viewTweet).toHaveBeenCalledTimes(1));
    expect(warpnetService.viewTweet).toHaveBeenCalledWith('t1', 'author1');

    await waitFor(() => {
      const eyeIcon = document.querySelector('.fa-eye');
      const counter = eyeIcon?.closest('div')?.querySelector('p');
      expect(counter?.textContent).toBe('7');
    });
  });

  it('records a view for a tall tweet via the fillsViewport branch', async () => {
    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    // Tall tweet: only 30% of the tweet is visible, but that 30% slice
    // covers the entire viewport (fillsViewport).
    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.3,
      intersectionRect: { height: 600 },
      rootBounds: { height: 600 },
    });

    await waitFor(() => expect(warpnetService.viewTweet).toHaveBeenCalledTimes(1));
  });

  it('does not record a view when the tweet is barely visible', async () => {
    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.1,
      intersectionRect: { height: 50 },
      rootBounds: { height: 800 },
    });

    await nextTick();
    await nextTick();
    expect(warpnetService.viewTweet).not.toHaveBeenCalled();
  });

  it('does not double-count when the observer fires multiple times', async () => {
    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    const fire = () =>
      observer.trigger({
        isIntersecting: true,
        intersectionRatio: 0.7,
        intersectionRect: { height: 400 },
        rootBounds: { height: 800 },
      });

    fire();
    fire();
    fire();

    await waitFor(() => expect(warpnetService.viewTweet).toHaveBeenCalledTimes(1));
  });

  it('retries the view request after a transient null response on the next intersection', async () => {
    let resolveFirst;
    warpnetService.viewTweet.mockImplementationOnce(
      () => new Promise((res) => { resolveFirst = res; })
    );

    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.7,
      intersectionRect: { height: 400 },
      rootBounds: { height: 800 },
    });

    await waitFor(() => expect(warpnetService.viewTweet).toHaveBeenCalledTimes(1));

    // First response: backend signaled a transient failure.
    resolveFirst(null);
    // Drain the recordView promise so viewInFlight is back to false.
    await new Promise((r) => setTimeout(r, 0));

    warpnetService.viewTweet.mockResolvedValueOnce(3);
    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.8,
      intersectionRect: { height: 500 },
      rootBounds: { height: 800 },
    });

    await waitFor(() => expect(warpnetService.viewTweet).toHaveBeenCalledTimes(2));
  });

  it('keeps the higher view count when loadTweetStats races with recordView', async () => {
    // Simulate the race: getTweetStats reads the count BEFORE the
    // viewTweet RecordView increment commits. recordView returns
    // post-increment 8, getTweetStats returns pre-increment 7.
    let resolveStats;
    warpnetService.getTweetStats.mockImplementationOnce(
      () => new Promise((res) => { resolveStats = res; })
    );
    warpnetService.viewTweet.mockResolvedValueOnce(8);

    renderTweet({ ...baseTweet });

    await waitFor(() => expect(observerInstances.length).toBeGreaterThan(0));
    const observer = observerInstances[0];

    observer.trigger({
      isIntersecting: true,
      intersectionRatio: 0.7,
      intersectionRect: { height: 400 },
      rootBounds: { height: 800 },
    });

    // recordView lands first with count=8.
    await waitFor(() => {
      const eyeIcon = document.querySelector('.fa-eye');
      const counter = eyeIcon?.closest('div')?.querySelector('p');
      expect(counter?.textContent).toBe('8');
    });

    // Now loadTweetStats lands with the older value 7 — must NOT clobber 8.
    resolveStats({
      tweet_id: 't1',
      tweets_count: 0,
      retweets_count: 0,
      likes_count: 0,
      replies_count: 0,
      views_count: 7,
    });
    await new Promise((r) => setTimeout(r, 0));
    await new Promise((r) => setTimeout(r, 0));

    const eyeIcon = document.querySelector('.fa-eye');
    const counter = eyeIcon?.closest('div')?.querySelector('p');
    expect(counter?.textContent).toBe('8');
  });
});
