/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_POST_TWEET_EDIT } from '@/service/service';
import { Call } from '@/lib/transport';

const owner = { user_id: 'owner1', username: 'Owner', node_id: 'node-owner' };

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue(owner);
  Call.mockResolvedValue({ body: {} });
});

describe('editTweet', () => {
  it('edits a tweet by its id alone', async () => {
    await warpnetService.editTweet('t1', 'new text');

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_TWEET_EDIT);
    expect(sent.body).toEqual({ tweet_id: 't1', user_id: 'owner1', text: 'new text' });
  });

  it('names the parent of a reply, which is stored under it', async () => {
    await warpnetService.editTweet('r1', 'new text', 'p1');

    expect(Call.mock.calls[0][0].body).toEqual({
      tweet_id: 'r1', user_id: 'owner1', text: 'new text', parent_id: 'p1',
    });
  });
});
