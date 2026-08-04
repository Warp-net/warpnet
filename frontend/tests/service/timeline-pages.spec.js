import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PUBLIC_GET_TWEETS, PUBLIC_GET_FOLLOWINGS } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({
    user_id: 'owner1',
    node_id: 'node-owner',
  });
});

describe('getUserTweetsPage', () => {
  it('passes the explicit cursor through and returns the next one', async () => {
    Call.mockResolvedValue({
      code: 200,
      body: { tweets: [{ id: 't1' }], cursor: 'next-cursor' },
    });

    const page = await warpnetService.getUserTweetsPage({
      userId: 'bob@mastodon.social',
      cursor: 'prev-cursor',
    });

    expect(Call).toHaveBeenCalledTimes(1);
    expect(Call.mock.calls[0][0].path).toBe(PUBLIC_GET_TWEETS);
    expect(Call.mock.calls[0][0].body).toMatchObject({
      user_id: 'bob@mastodon.social',
      cursor: 'prev-cursor',
      limit: 20,
    });
    expect(page).toEqual({ tweets: [{ id: 't1' }], cursor: 'next-cursor' });
  });

  it('maps a missing cursor to end and an end cursor to a no-op', async () => {
    Call.mockResolvedValue({ code: 200, body: { tweets: [] } });

    const page = await warpnetService.getUserTweetsPage({ userId: 'bob@x.y' });
    expect(page).toEqual({ tweets: [], cursor: 'end' });

    Call.mockClear();
    const done = await warpnetService.getUserTweetsPage({ userId: 'bob@x.y', cursor: 'end' });
    expect(done).toEqual({ tweets: [], cursor: 'end' });
    expect(Call).not.toHaveBeenCalled();
  });

  it('throws on an error-shaped body instead of reading it as exhaustion', async () => {
    // sendToNode maps an error response to {} — that must surface as a
    // retryable failure, not as an empty (exhausted) page.
    Call.mockResolvedValue({ code: 500, message: 'gateway down' });

    await expect(
      warpnetService.getUserTweetsPage({ userId: 'bob@x.y' }),
    ).rejects.toThrow(/no tweets response/);
  });

  it('leaves the global tweets cursor alone', async () => {
    warpnetService.setCursor('tweets', 'profile-view-cursor');
    Call.mockResolvedValue({
      code: 200,
      body: { tweets: [{ id: 't1' }], cursor: 'other' },
    });

    await warpnetService.getUserTweetsPage({ userId: 'bob@x.y' });

    expect(warpnetService.getCursor('tweets')).toBe('profile-view-cursor');
  });
});

describe('listFollowingIds', () => {
  it('paginates to the end and drops the self id', async () => {
    Call
      .mockResolvedValueOnce({
        code: 200,
        body: { followings: ['a', 'owner1', 'bob@mastodon.social'], cursor: 'page2' },
      })
      .mockResolvedValueOnce({
        code: 200,
        body: { followings: ['c'], cursor: 'end' },
      });

    const ids = await warpnetService.listFollowingIds('owner1');

    expect(ids).toEqual(['a', 'bob@mastodon.social', 'c']);
    expect(Call).toHaveBeenCalledTimes(2);
    expect(Call.mock.calls[0][0].path).toBe(PUBLIC_GET_FOLLOWINGS);
    expect(Call.mock.calls[0][0].body.cursor).toBe('');
    expect(Call.mock.calls[1][0].body.cursor).toBe('page2');
  });

  it('stops on an empty page and leaves the global followings cursor alone', async () => {
    warpnetService.setCursor('followings', 'following-view-cursor');
    Call.mockResolvedValue({ code: 200, body: { followings: [], cursor: 'more' } });

    const ids = await warpnetService.listFollowingIds('owner1');

    expect(ids).toEqual([]);
    expect(Call).toHaveBeenCalledTimes(1);
    expect(warpnetService.getCursor('followings')).toBe('following-view-cursor');
  });
});
