/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PUBLIC_GET_TWEET_STATS } from '@/service/service';
import { Call } from '@/lib/transport';

const owner = { user_id: 'owner1', username: 'Owner', node_id: 'node-owner' };
let seq = 0;
const freshId = () => `tweet-${++seq}`;
const statsCalls = () => Call.mock.calls.filter(([req]) => req.path === PUBLIC_GET_TWEET_STATS).length;

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue(owner);
  Call.mockImplementation(async (req) => ({ body: { tweet_id: req.body.tweet_id, replies_count: 1 } }));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('tweet stats window', () => {
  it('answers a repeated read from the last one', async () => {
    const id = freshId();
    const first = await warpnetService.getTweetStats(id, 'author');
    const second = await warpnetService.getTweetStats(id, 'author');

    expect(second).toEqual(first);
    expect(statsCalls()).toBe(1);
  });

  it('shares a read that is still in flight', async () => {
    const id = freshId();
    await Promise.all([
      warpnetService.getTweetStats(id, 'author'),
      warpnetService.getTweetStats(id, 'author'),
    ]);

    expect(statsCalls()).toBe(1);
  });

  it('reads again when forced', async () => {
    const id = freshId();
    await warpnetService.getTweetStats(id, 'author');
    await warpnetService.getTweetStats(id, 'author', true);

    expect(statsCalls()).toBe(2);
  });

  it('reads again once the window has passed', async () => {
    vi.useFakeTimers();
    const id = freshId();
    await warpnetService.getTweetStats(id, 'author');
    vi.advanceTimersByTime(30001);
    await warpnetService.getTweetStats(id, 'author');

    expect(statsCalls()).toBe(2);
  });

  it('does not keep an empty answer', async () => {
    const id = freshId();
    Call.mockResolvedValueOnce({ body: {} });
    await warpnetService.getTweetStats(id, 'author');
    const second = await warpnetService.getTweetStats(id, 'author');

    expect(second.tweet_id).toBe(id);
    expect(statsCalls()).toBe(2);
  });

  it('does not keep a failed read', async () => {
    const id = freshId();
    Call.mockRejectedValueOnce(new Error('stream reset'));
    await expect(warpnetService.getTweetStats(id, 'author')).rejects.toThrow('stream reset');
    await warpnetService.getTweetStats(id, 'author');

    expect(statsCalls()).toBe(2);
  });

  it('reads the parent and the root again after a reply', async () => {
    const parent = freshId();
    const root = freshId();
    await warpnetService.getTweetStats(parent, 'author');
    await warpnetService.getTweetStats(root, 'author');

    await warpnetService.replyTweet({ rootId: root, parentId: parent, parentUserId: 'author', text: 'hi' });
    await warpnetService.getTweetStats(parent, 'author');
    await warpnetService.getTweetStats(root, 'author');

    expect(statsCalls()).toBe(4);
  });
});
