import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getVideo: vi.fn(),
    getSponsoredImage: vi.fn(),
    getSponsoredVideo: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    hasReactor: vi.fn(),
    getReactorEmoji: vi.fn(),
    hasRetweeter: vi.fn(),
    viewTweet: vi.fn(),
    isBookmarked: vi.fn(),
    orderSponsoredTweet: vi.fn(),
    getSponsoredTweet: vi.fn(),
    quoteSponsoredTweet: vi.fn(),
    getCopyBuyer: vi.fn(),
  },
}));

vi.mock('@/lib/order-worker', () => ({ watchOrder: vi.fn() }));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';
import { watchOrder } from '@/lib/order-worker';

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

const quote = (over = {}) => ({
  token: 'TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf',
  fee_percent: 5,
  fee: '75000',
  total: '1575000',
  balance: '3000000',
  trx: '25000000',
  energy_price: 100,
  bandwidth_price: 1000,
  network_fee: '6726600',
  steps: [
    { kind: 'approve', energy: 0, bandwidth: 345, burn: '0', approximate: false },
    { kind: 'pay', energy: 63156, bandwidth: 411, burn: '6726600', approximate: true },
  ],
  ...over,
});

const openBill = async () => {
  await fireEvent.click(await screen.findByRole('button', { name: 'Unlock for 1.5 USDT' }));
  return screen.findByRole('table', { name: 'What the tweet costs' });
};

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
  warpnetService.getSponsoredImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'viewer1', node_id: 'node-viewer' });
  warpnetService.getTweetStats.mockResolvedValue({ tweet_id: 't1' });
  warpnetService.hasReactor.mockResolvedValue(false);
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.isBookmarked?.mockResolvedValue?.(false);
  warpnetService.getSponsoredTweet.mockResolvedValue(null);
  warpnetService.quoteSponsoredTweet.mockResolvedValue(quote());
});

describe('TweetBlock sponsored tweet', () => {
  it('shows a teaser as a locked card with its price', async () => {
    renderTweet(teaser());

    expect(await screen.findByRole('img', { name: 'Locked tweet' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unlock for 1.5 USDT' })).toBeInTheDocument();
    await waitFor(() => expect(warpnetService.getSponsoredTweet).toHaveBeenCalledWith({ tweetId: 't1', userId: 'author1' }));
  });

  it('pays after the confirmation and shows what was bought', async () => {
    warpnetService.orderSponsoredTweet.mockResolvedValue({ tweet_id: 't1', tx_id: 'tx1', confirmed: true });
    renderTweet(teaser());
    await waitFor(() => expect(warpnetService.getSponsoredTweet).toHaveBeenCalledTimes(1));
    warpnetService.getSponsoredTweet.mockResolvedValue({ ...teaser(), text: 'the paid words' });

    await openBill();
    expect(warpnetService.orderSponsoredTweet).not.toHaveBeenCalled();
    await fireEvent.click(screen.getByRole('button', { name: 'Pay' }));

    await waitFor(() => expect(screen.getByText('the paid words')).toBeInTheDocument());
    expect(warpnetService.orderSponsoredTweet).toHaveBeenCalledWith({ tweetId: 't1', userId: 'author1' });
    expect(screen.queryByRole('img', { name: 'Locked tweet' })).not.toBeInTheDocument();
  });

  it('leaves an unconfirmed payment to the order worker and opens once it confirms', async () => {
    warpnetService.orderSponsoredTweet.mockResolvedValue({ tweet_id: 't1', tx_id: 'tx1', confirmed: false });
    let unlocked;
    watchOrder.mockImplementation((_tweetId, _userId, onUnlocked) => {
      unlocked = onUnlocked;
      return vi.fn();
    });
    renderTweet(teaser());

    await openBill();
    await fireEvent.click(screen.getByRole('button', { name: 'Pay' }));

    expect(await screen.findByRole('status')).toHaveTextContent(/confirming it, the tweet opens by itself/);
    expect(screen.queryByRole('button', { name: /Unlock for/ })).not.toBeInTheDocument();
    expect(watchOrder).toHaveBeenCalledWith('t1', 'author1', expect.any(Function));

    unlocked({ ...teaser(), text: 'confirmed at last' });

    expect(await screen.findByText('confirmed at last')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(warpnetService.orderSponsoredTweet).toHaveBeenCalledTimes(1);
  });

  it('shows the whole bill before paying', async () => {
    renderTweet(teaser());
    const bill = within(await openBill());

    expect(warpnetService.quoteSponsoredTweet).toHaveBeenCalledWith({ tweetId: 't1', userId: 'author1' });
    expect(bill.getByText('To author')).toBeInTheDocument();
    expect(bill.getByText('1.5 USDT')).toBeInTheDocument();
    expect(bill.getByText('Service fee, 5%')).toBeInTheDocument();
    expect(bill.getByText('0.075 USDT')).toBeInTheDocument();
    expect(bill.getByText('1.575 USDT')).toBeInTheDocument();
    expect(bill.getByText('Network fee')).toBeInTheDocument();
    expect(bill.getByText('≈ 6.7266 TRX')).toBeInTheDocument();
    expect(bill.queryByText(/Approving|Payment|energy|bytes/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pay' })).toBeEnabled();
    expect(screen.queryByText(/not enough|less than the network fee/)).not.toBeInTheDocument();
  });

  it('will not pay without enough USDT', async () => {
    warpnetService.quoteSponsoredTweet.mockResolvedValue(quote({ balance: '1000000' }));
    renderTweet(teaser());
    await openBill();

    expect(screen.getByText('You have 1 USDT, not enough to pay.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pay' })).toBeDisabled();
  });

  it('warns when the TRX may not cover the network fee', async () => {
    warpnetService.quoteSponsoredTweet.mockResolvedValue(quote({ trx: '2000000' }));
    renderTweet(teaser());
    await openBill();

    expect(screen.getByText('You have 2 TRX, less than the network fee.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pay' })).toBeEnabled();
  });

  it('still lets the reader pay when the cost cannot be worked out', async () => {
    warpnetService.quoteSponsoredTweet.mockRejectedValue(new Error('wallet: payment engine unavailable'));
    warpnetService.orderSponsoredTweet.mockResolvedValue({ tweet_id: 't1', tx_id: 'tx1', confirmed: false });
    watchOrder.mockReturnValue(vi.fn());
    renderTweet(teaser());
    await fireEvent.click(await screen.findByRole('button', { name: 'Unlock for 1.5 USDT' }));

    expect(await screen.findByText("Couldn't work out the network fee: wallet: payment engine unavailable")).toBeInTheDocument();
    expect(screen.getByText(/plus a service fee of up to 5% and a TRX network fee/)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Pay' }));
    expect(warpnetService.orderSponsoredTweet).toHaveBeenCalledTimes(1);
  });

  it('comes back to a confirming payment without offering to pay again', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValue({ pending: true });
    const unwatch = vi.fn();
    watchOrder.mockReturnValue(unwatch);
    const { unmount } = renderTweet(teaser());

    expect(await screen.findByRole('status')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Unlock for/ })).not.toBeInTheDocument();
    expect(watchOrder).toHaveBeenCalledTimes(1);

    unmount();
    expect(unwatch).toHaveBeenCalled();
  });

  it('opens a tweet bought earlier without asking again', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValue({ ...teaser(), text: 'bought before' });
    renderTweet(teaser());

    expect(await screen.findByText('bought before')).toBeInTheDocument();
    expect(warpnetService.orderSponsoredTweet).not.toHaveBeenCalled();
  });

  it("loads a bought tweet's media through the sponsored routes", async () => {
    warpnetService.getSponsoredTweet.mockResolvedValue({ ...teaser(), text: 'bought before', image_keys: ['copy-1'], video_key: 'copy-2' });
    warpnetService.getSponsoredVideo.mockResolvedValue({ file: 'data:video/mp4;base64,AAAA', size: 4, deferred: false });
    renderTweet(teaser());

    await waitFor(() => expect(warpnetService.getSponsoredImage).toHaveBeenCalledWith({ userId: 'author1', key: 'copy-1' }));
    await fireEvent.click(await screen.findByLabelText('Play video'));
    await waitFor(() => expect(warpnetService.getSponsoredVideo).toHaveBeenCalledWith({ userId: 'author1', key: 'copy-2' }));
    expect(warpnetService.getImage).not.toHaveBeenCalledWith(expect.objectContaining({ key: 'copy-1' }));
    expect(warpnetService.getVideo).not.toHaveBeenCalled();
  });

  it("loads the author's own sponsored media as the originals", async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1', node_id: 'node-author' });
    renderTweet({ ...teaser(), text: 'my paid words', image_keys: ['original-1'] });

    await waitFor(() => expect(warpnetService.getImage).toHaveBeenCalledWith({ userId: 'author1', key: 'original-1' }));
    expect(warpnetService.getSponsoredImage).not.toHaveBeenCalled();
  });

  it("shows the author's own sponsored tweet in full, with its price", async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1', node_id: 'node-author' });
    renderTweet({ ...teaser(), text: 'my paid words' });

    expect(await screen.findByText('my paid words')).toBeInTheDocument();
    expect(screen.getByText(/1\.5 USDT/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Unlock/ })).not.toBeInTheDocument();
    expect(warpnetService.getSponsoredTweet).not.toHaveBeenCalled();
  });

  it('names the buyer of a copy the author checks', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1', node_id: 'node-author' });
    warpnetService.getProfile.mockImplementation(async (id) =>
      id === 'buyer1' ? { id, username: 'leaker' } : { id, username: 'author', avatar_key: '' });
    warpnetService.getCopyBuyer.mockResolvedValue({
      tweet_id: 't1', buyer_id: 'buyer1', order_id: 'order-hash', tx_id: 'tx1', sold_at: '2026-10-01T12:00:00Z',
      daily_orders_count: 7,
    });
    renderTweet({ ...teaser(), text: 'my paid words' });

    await fireEvent.click(await screen.findByRole('button', { name: 'Tweet options' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Check a copy' }));
    const leaked = new File([new Uint8Array([0xff, 0xd8, 0xff])], 'leak.jpg', { type: 'image/jpeg' });
    await fireEvent.change(screen.getByLabelText('Copy to check'), { target: { files: [leaked] } });

    expect(await screen.findByText(/Sold to leaker \(@buyer1\)/)).toBeInTheDocument();
    expect(screen.getByText(/Order order-hash/)).toBeInTheDocument();
    expect(screen.getByText(/bought 7 of your tweets within a day of this one/)).toBeInTheDocument();
    expect(warpnetService.getCopyBuyer).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/jpeg;base64,/));
  });

  it('says so when a checked file names no buyer', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'author1', node_id: 'node-author' });
    warpnetService.getCopyBuyer.mockResolvedValue(null);
    renderTweet({ ...teaser(), text: 'my paid words' });

    await fireEvent.click(await screen.findByRole('button', { name: 'Tweet options' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Check a copy' }));
    const screenshot = new File(['x'], 'shot.png', { type: 'image/png' });
    await fireEvent.change(screen.getByLabelText('Copy to check'), { target: { files: [screenshot] } });

    expect(await screen.findByText(/names no buyer/)).toBeInTheDocument();
  });

  it('offers no copy check to a reader', async () => {
    renderTweet(teaser());

    await fireEvent.click(await screen.findByRole('button', { name: 'Tweet options' }));
    expect(screen.queryByRole('button', { name: 'Check a copy' })).not.toBeInTheDocument();
  });
});
