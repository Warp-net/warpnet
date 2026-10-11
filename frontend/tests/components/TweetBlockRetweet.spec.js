import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

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
    retweetTweet: vi.fn(),
    unretweetTweet: vi.fn(),
    setRetweeter: vi.fn(),
    deleteRetweeter: vi.fn(),
  },
}));

vi.mock('@/lib/toast', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';
import { toast } from '@/lib/toast';

class FakeIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const owner = { user_id: 'viewer1', node_id: 'node-viewer' };

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
  warpnetService.getProfile.mockResolvedValue({ id: 'author1', username: 'author', avatar_key: '' });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue(owner);
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.hasBookmark.mockResolvedValue(false);
  warpnetService.viewTweet.mockResolvedValue(0);
  warpnetService.retweetTweet.mockResolvedValue({});
  warpnetService.unretweetTweet.mockResolvedValue({});
  warpnetService.setRetweeter.mockResolvedValue();
  warpnetService.deleteRetweeter.mockResolvedValue();
  warpnetService.getTweetStats.mockResolvedValue({
    tweet_id: 't1',
    retweets_count: 0,
    reactions_count: 0,
    replies_count: 0,
    views_count: 0,
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

const renderTweet = async (overrides = {}) => {
  const view = render(TweetBlock, {
    props: { tweet: { ...baseTweet, ...overrides } },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
    },
  });
  await waitFor(() => expect(warpnetService.hasBookmark).toHaveBeenCalled());
  return view;
};

const retweetButton = () =>
  [...document.querySelectorAll('button')].find((b) =>
    ['Retweet', 'Undo retweet'].includes(b.getAttribute('aria-label')));

const menuItem = (text) =>
  [...document.querySelectorAll('button')].find((b) => !b.getAttribute('aria-label') && b.textContent.trim() === text);

const waitForLabel = (label) =>
  waitFor(() => expect(retweetButton().getAttribute('aria-label')).toBe(label));

describe('TweetBlock retweet button', () => {
  it('retweets from the menu and remembers it', async () => {
    await renderTweet();
    await waitForLabel('Retweet');

    await fireEvent.click(retweetButton());
    await fireEvent.click(menuItem('Retweet'));

    await waitFor(() => expect(warpnetService.retweetTweet).toHaveBeenCalledWith({
      tweetId: 't1',
      userId: 'author1',
      username: 'author',
      text: 'hello world',
    }));
    expect(warpnetService.setRetweeter).toHaveBeenCalledWith('t1', owner.user_id, owner);
    expect(warpnetService.unretweetTweet).not.toHaveBeenCalled();
    await waitForLabel('Undo retweet');
  });

  it('undoes the viewer’s retweet in one click', async () => {
    await renderTweet({ retweeted_by: owner.user_id });
    await waitForLabel('Undo retweet');

    await fireEvent.click(retweetButton());

    await waitFor(() => expect(warpnetService.unretweetTweet).toHaveBeenCalledWith('t1'));
    expect(warpnetService.deleteRetweeter).toHaveBeenCalledWith('t1', owner.user_id);
    expect(menuItem('Retweet')).toBeUndefined();
    await waitForLabel('Retweet');
  });

  it('undoes a self-retweet from its RT: card', async () => {
    await renderTweet({ id: 'RT:t1', user_id: owner.user_id, retweeted_by: owner.user_id });
    await waitForLabel('Undo retweet');

    await fireEvent.click(retweetButton());

    await waitFor(() => expect(warpnetService.unretweetTweet).toHaveBeenCalledWith('RT:t1'));
    expect(warpnetService.deleteRetweeter).toHaveBeenCalledWith('RT:t1', owner.user_id);
    await waitForLabel('Retweet');
  });

  it('paints a retweet remembered in the cache', async () => {
    warpnetService.hasRetweeter.mockResolvedValue(true);
    await renderTweet();

    await waitForLabel('Undo retweet');
    expect(warpnetService.hasRetweeter).toHaveBeenCalledWith('t1', owner.user_id);
  });

  it('keeps the retweet when the node refuses to undo it', async () => {
    warpnetService.unretweetTweet.mockRejectedValue(new Error('tweet not found'));
    await renderTweet({ retweeted_by: owner.user_id });
    await waitForLabel('Undo retweet');

    await fireEvent.click(retweetButton());

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('tweet not found'));
    expect(warpnetService.deleteRetweeter).not.toHaveBeenCalled();
    await waitForLabel('Undo retweet');
  });

  it('drops the retweet when the node refuses it', async () => {
    warpnetService.retweetTweet.mockRejectedValue(new Error('offline'));
    await renderTweet();
    await waitForLabel('Retweet');

    await fireEvent.click(retweetButton());
    await fireEvent.click(menuItem('Retweet'));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('offline'));
    expect(warpnetService.setRetweeter).not.toHaveBeenCalled();
    await waitForLabel('Retweet');
  });

  it('refreshes the counters after the toggle', async () => {
    await renderTweet({ retweeted_by: owner.user_id });
    await waitForLabel('Undo retweet');
    const before = warpnetService.getTweetStats.mock.calls.length;

    await fireEvent.click(retweetButton());

    await waitFor(() => expect(warpnetService.getTweetStats.mock.calls.length).toBeGreaterThan(before));
  });
});
