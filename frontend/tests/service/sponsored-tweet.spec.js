/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_POST_TWEET, PRIVATE_POST_SPONSORED_TWEET } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', username: 'Owner', node_id: 'node-owner' });
});

describe('createTweet with a price', () => {
  it('goes to the sponsored route and carries the price', async () => {
    Call.mockResolvedValue({ body: { id: 't1' } });

    await warpnetService.createTweet({ text: 'members only', price: '1500000' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_SPONSORED_TWEET);
    expect(sent.body.price).toEqual({ units: 1500000 });
  });

  it('leaves a free tweet on the plain route', async () => {
    Call.mockResolvedValue({ body: { id: 't1' } });

    await warpnetService.createTweet({ text: 'hello' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_TWEET);
    expect(sent.body).not.toHaveProperty('price');
  });

  // No node serves the route yet, and its refusal must not read as posted.
  it.each([
    ['an error body', { body: { code: 500, message: 'protocols not supported' } }],
    ['no body at all', { code: 500, message: 'protocols not supported' }],
  ])('rejects when the node answers with %s', async (_, answer) => {
    Call.mockResolvedValue(answer);

    await expect(warpnetService.createTweet({ text: 'members only', price: '1500000' }))
      .rejects.toThrow("can't publish sponsored tweets");
  });
});
