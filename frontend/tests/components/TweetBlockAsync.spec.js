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
    isTrusted: vi.fn(),
    isStrangerMediaShown: vi.fn(),
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
  warpnetService.isTrusted.mockResolvedValue(true);
  warpnetService.isStrangerMediaShown.mockReturnValue(false);
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
  it("names the author by their current profile, not the name stored in the tweet", async () => {
    warpnetService.getProfile.mockResolvedValue({ id: 'author1', username: 'renamed', avatar_key: '' });

    renderTweet({ ...baseTweet });

    expect(await screen.findByText('renamed')).toBeInTheDocument();
    expect(screen.queryByText('author')).not.toBeInTheDocument();
  });

  it('keeps the name stored in the tweet while the author profile hangs', async () => {
    warpnetService.getProfile.mockImplementation(() => new Promise(() => {}));

    renderTweet({ ...baseTweet });

    expect(await screen.findByText('author')).toBeInTheDocument();
  });

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

  it('does not fetch a stranger\'s images until the owner asks', async () => {
    warpnetService.isTrusted.mockResolvedValue(false);
    warpnetService.getImage.mockResolvedValue('data:image/png;base64,one');

    renderTweet({ ...baseTweet, image_keys: ['k1'] });

    const show = await screen.findByText('Show');
    expect(screen.getByText('Media from an unfamiliar author')).toBeInTheDocument();
    expect(warpnetService.getImage).not.toHaveBeenCalledWith(expect.objectContaining({ key: 'k1' }));
    expect(warpnetService.isTrusted).toHaveBeenCalledWith('author1');

    show.click();
    await waitFor(() => {
      expect(screen.getByAltText('Tweet image')).toHaveAttribute('src', 'data:image/png;base64,one');
    });
  });

  it('shows a stranger\'s images at once when the owner opted in', async () => {
    warpnetService.isStrangerMediaShown.mockReturnValue(true);
    warpnetService.isTrusted.mockResolvedValue(false);
    warpnetService.getImage.mockResolvedValue('data:image/png;base64,one');

    renderTweet({ ...baseTweet, image_keys: ['k1'] });

    expect(await screen.findByAltText('Tweet image')).toHaveAttribute('src', 'data:image/png;base64,one');
  });

  it('awards a trusted author the medal', async () => {
    renderTweet(baseTweet);
    expect(await screen.findByRole('img', { name: 'In your circle' })).toBeInTheDocument();
  });

  it('awards no medal to a stranger or when the node did not answer', async () => {
    for (const answer of [false, null]) {
      warpnetService.isTrusted.mockResolvedValue(answer);
      const { unmount } = renderTweet(baseTweet);
      await waitFor(() => expect(warpnetService.isTrusted).toHaveBeenCalled());
      await Promise.resolve();
      expect(screen.queryByRole('img', { name: 'In your circle' })).toBeNull();
      unmount();
      warpnetService.isTrusted.mockClear();
    }
  });

  it('shows media at once when the node did not answer', async () => {
    warpnetService.isTrusted.mockResolvedValue(null);
    warpnetService.getImage.mockResolvedValue('data:image/png;base64,one');

    renderTweet({ ...baseTweet, image_keys: ['k1'] });

    expect(await screen.findByAltText('Tweet image')).toHaveAttribute('src', 'data:image/png;base64,one');
  });

  it('awards no medal on the owner\'s own tweet', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1' });
    renderTweet(baseTweet);
    await screen.findByText('hello world');
    expect(warpnetService.isTrusted).not.toHaveBeenCalled();
    expect(screen.queryByRole('img', { name: 'In your circle' })).toBeNull();
  });
});
