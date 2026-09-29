import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    hasReactor: vi.fn(),
    getReactorEmoji: vi.fn(),
    hasRetweeter: vi.fn(),
    viewTweet: vi.fn(),
    isBookmarked: vi.fn(),
    purchaseSponsoredTweet: vi.fn(),
    getSponsoredTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

class FakeIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const teaser = () => ({
  id: 't1',
  user_id: 'author1',
  username: 'author',
  text: '',
  created_at: '2026-09-29T10:00:00Z',
  price: { amount: '1.5', units: 1500000 },
});

const renderTweet = (tweet) =>
  render(TweetBlock, {
    props: { tweet },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
      directives: { linkify: {} },
    },
  });

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  warnSpy.mockRestore();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getProfile.mockResolvedValue({ id: 'author1', username: 'author', avatar_key: '' });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'viewer1', node_id: 'node-viewer' });
  warpnetService.getTweetStats.mockResolvedValue({ tweet_id: 't1' });
  warpnetService.hasReactor.mockResolvedValue(false);
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.isBookmarked?.mockResolvedValue?.(false);
  warpnetService.getSponsoredTweet.mockResolvedValue(null);
});

describe('TweetBlock sponsored tweet', () => {
  it('shows a teaser as a locked card with its price', async () => {
    renderTweet(teaser());

    expect(await screen.findByRole('img', { name: 'Locked tweet' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unlock for 1.5 USDT' })).toBeInTheDocument();
    await waitFor(() => expect(warpnetService.getSponsoredTweet).toHaveBeenCalledWith({ tweetId: 't1', userId: 'author1' }));
  });

  it('pays after the confirmation and shows what was bought', async () => {
    warpnetService.purchaseSponsoredTweet.mockResolvedValue({ tweet_id: 't1', tx_id: 'tx1', confirmed: true });
    renderTweet(teaser());
    await waitFor(() => expect(warpnetService.getSponsoredTweet).toHaveBeenCalledTimes(1));
    warpnetService.getSponsoredTweet.mockResolvedValue({ ...teaser(), text: 'the paid words' });

    await fireEvent.click(screen.getByRole('button', { name: 'Unlock for 1.5 USDT' }));
    expect(warpnetService.purchaseSponsoredTweet).not.toHaveBeenCalled();
    await fireEvent.click(await screen.findByRole('button', { name: 'Pay' }));

    await waitFor(() => expect(screen.getByText('the paid words')).toBeInTheDocument());
    expect(warpnetService.purchaseSponsoredTweet).toHaveBeenCalledWith({ tweetId: 't1', userId: 'author1' });
    expect(screen.queryByRole('img', { name: 'Locked tweet' })).not.toBeInTheDocument();
  });

  it('says so when the network has not confirmed the payment yet', async () => {
    warpnetService.purchaseSponsoredTweet.mockResolvedValue({ tweet_id: 't1', tx_id: 'tx1', confirmed: false });
    renderTweet(teaser());

    await fireEvent.click(await screen.findByRole('button', { name: 'Unlock for 1.5 USDT' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Pay' }));

    expect(await screen.findByText(/still confirming/)).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Locked tweet' })).toBeInTheDocument();
  });

  it('opens a tweet bought earlier without asking again', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValue({ ...teaser(), text: 'bought before' });
    renderTweet(teaser());

    expect(await screen.findByText('bought before')).toBeInTheDocument();
    expect(warpnetService.purchaseSponsoredTweet).not.toHaveBeenCalled();
  });

  it("shows the author's own sponsored tweet in full, with its price", async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1', node_id: 'node-author' });
    renderTweet({ ...teaser(), text: 'my paid words' });

    expect(await screen.findByText('my paid words')).toBeInTheDocument();
    expect(screen.getByText(/1\.5 USDT/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Unlock/ })).not.toBeInTheDocument();
    expect(warpnetService.getSponsoredTweet).not.toHaveBeenCalled();
  });
});
