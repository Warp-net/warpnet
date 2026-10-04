import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('@/service/service', () => ({ warpnetService: { getSponsoredTweet: vi.fn() } }));
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }));

import { watchOrder } from '@/lib/order-worker';
import { warpnetService } from '@/service/service';
import { toast } from '@/lib/toast';

const paid = (id) => ({ id, user_id: 'author1', text: 'the paid words' });

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.spyOn(console, 'warn').mockImplementation(() => {});
});
afterEach(() => {
  vi.useRealTimers();
});

describe('order worker', () => {
  it('asks the node until the payment confirms, then hands the tweet over', async () => {
    warpnetService.getSponsoredTweet
      .mockResolvedValueOnce({ pending: true })
      .mockRejectedValueOnce(new Error('node unreachable'))
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(paid('w1'));
    const onUnlocked = vi.fn();

    watchOrder('w1', 'author1', onUnlocked);
    await vi.advanceTimersByTimeAsync(4999);
    expect(warpnetService.getSponsoredTweet).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);

    expect(warpnetService.getSponsoredTweet).toHaveBeenCalledTimes(4);
    expect(warpnetService.getSponsoredTweet).toHaveBeenCalledWith({ tweetId: 'w1', userId: 'author1' });
    expect(onUnlocked).toHaveBeenCalledWith(paid('w1'));
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('runs one wait per tweet however many cards watch it', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValueOnce({ pending: true }).mockResolvedValueOnce(paid('w2'));
    const first = vi.fn();
    const second = vi.fn();

    watchOrder('w2', 'author1', first);
    watchOrder('w2', 'author1', second);
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);

    expect(warpnetService.getSponsoredTweet).toHaveBeenCalledTimes(2);
    expect(first).toHaveBeenCalledWith(paid('w2'));
    expect(second).toHaveBeenCalledWith(paid('w2'));
  });

  it('backs off to a check a minute', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValue({ pending: true });

    watchOrder('w3', 'author1', vi.fn());
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000);
    const checks = warpnetService.getSponsoredTweet.mock.calls.length;
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000);

    expect(warpnetService.getSponsoredTweet.mock.calls.length - checks).toBe(10);
  });

  it('tells the reader when a payment confirms after they left the tweet', async () => {
    warpnetService.getSponsoredTweet.mockResolvedValueOnce({ pending: true }).mockResolvedValueOnce(paid('w4'));
    const onUnlocked = vi.fn();

    const unwatch = watchOrder('w4', 'author1', onUnlocked);
    await vi.advanceTimersByTimeAsync(5000);
    unwatch();
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);

    expect(onUnlocked).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith('Payment confirmed. The sponsored tweet is unlocked.');
  });
});
