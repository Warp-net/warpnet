import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    getLikerReaction: vi.fn(),
    hasRetweeter: vi.fn(),
    hasBookmark: vi.fn(),
    setLiker: vi.fn(),
    deleteLiker: vi.fn(),
    likeTweet: vi.fn(),
    unlikeTweet: vi.fn(),
    viewTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

class FakeIntersectionObserver {
  constructor(callback) {
    this.callback = callback;
  }
  observe() {}
  unobserve() {}
  disconnect() {}
}

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
  warpnetService.getProfile.mockResolvedValue({id: 'author1', username: 'author', avatar_key: ''});
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({user_id: 'viewer1', node_id: 'node-viewer'});
  warpnetService.getLikerReaction.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.hasBookmark.mockResolvedValue(false);
  warpnetService.viewTweet.mockResolvedValue(0);
  warpnetService.getTweetStats.mockResolvedValue({
    tweet_id: 't1',
    retweets_count: 0,
    likes_count: 3,
    replies_count: 0,
    views_count: 0,
    reactions: {'🔥': 2, '❤️': 1},
    my_reaction: '🔥',
  });
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

const renderTweet = () =>
  render(TweetBlock, {
    props: {tweet: {...baseTweet}},
    global: {
      mocks: {
        $filters: {timeago: () => 'just now'},
        $router: {push: vi.fn()},
      },
    },
  });

const chipFor = (emoji) =>
  [...document.querySelectorAll('button[aria-pressed]')].find((b) => b.textContent.includes(emoji));

describe('TweetBlock reactions', () => {
  it('renders one chip per emoji and marks the viewer’s own reaction', async () => {
    renderTweet();

    await waitFor(() => expect(chipFor('🔥')).toBeTruthy());
    expect(chipFor('🔥').textContent.replace(/\s/g, '')).toBe('🔥2');
    expect(chipFor('🔥').getAttribute('aria-pressed')).toBe('true');
    expect(chipFor('❤️').textContent.replace(/\s/g, '')).toBe('❤️1');
    expect(chipFor('❤️').getAttribute('aria-pressed')).toBe('false');
  });

  it('reacts with the chip’s emoji when another chip is clicked', async () => {
    warpnetService.likeTweet.mockResolvedValue({count: 3, reactions: {'🔥': 1, '❤️': 2}});
    renderTweet();

    await waitFor(() => expect(chipFor('❤️')).toBeTruthy());
    await fireEvent.click(chipFor('❤️'));

    await waitFor(() => expect(warpnetService.likeTweet).toHaveBeenCalledWith('t1', 'author1', '❤️'));
    expect(warpnetService.unlikeTweet).not.toHaveBeenCalled();
    await waitFor(() => expect(chipFor('❤️').getAttribute('aria-pressed')).toBe('true'));
  });

  it('takes the reaction back when the viewer clicks the one they hold', async () => {
    warpnetService.unlikeTweet.mockResolvedValue({count: 2, reactions: {'🔥': 1, '❤️': 1}});
    renderTweet();

    await waitFor(() => expect(chipFor('🔥')).toBeTruthy());
    await fireEvent.click(chipFor('🔥'));

    await waitFor(() => expect(warpnetService.unlikeTweet).toHaveBeenCalledWith('t1', 'author1'));
    expect(warpnetService.likeTweet).not.toHaveBeenCalled();
  });

  it('lets the node’s my_reaction outrank a stale local cache', async () => {
    // The cache says heart, the node says fire: the node wins, whichever
    // promise settles first.
    warpnetService.getLikerReaction.mockResolvedValue('❤️');
    renderTweet();

    await waitFor(() => expect(chipFor('🔥')?.getAttribute('aria-pressed')).toBe('true'));
    const button = [...document.querySelectorAll('button')]
      .find((el) => el.getAttribute('aria-label') === 'Remove reaction');
    expect(button.textContent.trim()).toBe('🔥');
  });

  it('reacts with a heart when the button is clicked with no reaction held', async () => {
    warpnetService.getTweetStats.mockResolvedValue({
      tweet_id: 't1',
      retweets_count: 0,
      likes_count: 0,
      replies_count: 0,
      views_count: 0,
      my_reaction: '',
    });
    warpnetService.likeTweet.mockResolvedValue({count: 1, reactions: {'❤️': 1}});
    renderTweet();

    const button = await waitFor(() => {
      const b = [...document.querySelectorAll('button')].find((el) => el.getAttribute('aria-label') === 'React');
      expect(b).toBeTruthy();
      return b;
    });
    await fireEvent.click(button);

    await waitFor(() => expect(warpnetService.likeTweet).toHaveBeenCalledWith('t1', 'author1', '❤️'));
    await waitFor(() => expect(chipFor('❤️')).toBeTruthy());
  });
});
