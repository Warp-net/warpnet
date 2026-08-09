import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    getReactorEmoji: vi.fn(),
    hasRetweeter: vi.fn(),
    hasBookmark: vi.fn(),
    viewTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

const renderTweet = (tweet) =>
  render(TweetBlock, {
    props: { tweet },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
    },
  });

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  warnSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getProfile.mockResolvedValue({ id: 'author1', username: 'author', avatar_key: '' });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'viewer1' });
  warpnetService.getTweetStats.mockResolvedValue({
    tweet_id: 't1',
    retweets_count: 0,
    reactions_count: 0,
    replies_count: 0,
    views_count: 0,
  });
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.hasBookmark.mockResolvedValue(false);
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

describe('TweetBlock per-element loading', () => {
  it('renders the tweet text without waiting for a hanging author profile', async () => {
    warpnetService.getProfile.mockImplementation(() => new Promise(() => {}));

    renderTweet({ ...baseTweet });

    expect(await screen.findByText('hello world')).toBeInTheDocument();
  });

  it('shows counters even when the profile and image blobs hang', async () => {
    warpnetService.getProfile.mockImplementation(() => new Promise(() => {}));
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));
    warpnetService.getTweetStats.mockResolvedValue({
      tweet_id: 't1',
      retweets_count: 0,
      reactions_count: 0,
      replies_count: 5,
      views_count: 0,
    });

    renderTweet({ ...baseTweet, image_keys: ['k1'] });

    expect(await screen.findByText('5')).toBeInTheDocument();
  });

  it('fills each image in place independently of a hanging sibling blob', async () => {
    warpnetService.getImage.mockImplementation(({ key }) =>
      key === 'k1' ? Promise.resolve('data:image/png;base64,one') : new Promise(() => {})
    );

    renderTweet({ ...baseTweet, image_keys: ['k1', 'k2'] });

    await waitFor(() => {
      const imgs = screen.getAllByAltText('Tweet image');
      expect(imgs).toHaveLength(1);
      expect(imgs[0]).toHaveAttribute('src', 'data:image/png;base64,one');
    });
  });
});
