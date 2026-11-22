/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_GET_TRUST } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
});

describe('isTrusted', () => {
  it('batches authors asked for in one tick into one request', async () => {
    Call.mockResolvedValue({ body: { users: {
      a1: { is_trusted: true, score: 1, vouchers: 0 },
      a2: { is_trusted: false, score: 0, vouchers: 0 },
    } } });

    const [a1, a2, again] = await Promise.all([
      warpnetService.isTrusted('a1'),
      warpnetService.isTrusted('a2'),
      warpnetService.isTrusted('a1'),
    ]);

    expect([a1, a2, again]).toEqual([true, false, true]);
    expect(Call).toHaveBeenCalledTimes(1);
    // Mirrors event.PRIVATE_GET_TRUST in event/paths.go.
    expect(PRIVATE_GET_TRUST).toBe('/private/get/trust/0.0.0');
    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_GET_TRUST);
    expect(sent.body).toEqual({ user_ids: ['a1', 'a2'] });
  });

  it('answers a repeated author from the cache', async () => {
    expect(await warpnetService.isTrusted('a2')).toBe(false);
    expect(Call).not.toHaveBeenCalled();
  });

  it('answers null for a failed request and does not cache it', async () => {
    Call.mockRejectedValueOnce(new Error('offline'));
    expect(await warpnetService.isTrusted('b1')).toBeNull();

    Call.mockResolvedValue({ body: { users: { b1: { is_trusted: false } } } });
    expect(await warpnetService.isTrusted('b1')).toBe(false);
  });
});

describe('isUnfamiliar', () => {
  beforeEach(() => localStorage.clear());

  it('is a stranger outside the circle, never an accepted chat or an unanswered lookup', async () => {
    Call.mockResolvedValue({ body: { users: {
      c1: { is_trusted: false },
      c2: { is_trusted: true },
      c3: { is_trusted: false },
    } } });
    warpnetService.acceptChat('c3');

    expect(await warpnetService.isUnfamiliar('c1')).toBe(true);
    expect(await warpnetService.isUnfamiliar('c2')).toBe(false);
    expect(await warpnetService.isUnfamiliar('c3')).toBe(false);

    Call.mockRejectedValueOnce(new Error('offline'));
    expect(await warpnetService.isUnfamiliar('c4')).toBe(false);
  });
});

describe('circleFirst', () => {
  it('moves the circle ahead and keeps the order within each part', async () => {
    Call.mockResolvedValue({ body: { users: {
      d1: { is_trusted: false },
      d2: { is_trusted: true },
      d3: { is_trusted: false },
      d4: { is_trusted: true },
    } } });

    const ordered = await warpnetService.circleFirst([{ id: 'd1' }, { id: 'd2' }, { id: 'd3' }, { id: 'd4' }]);
    expect(ordered.map((u) => u.id)).toEqual(['d2', 'd4', 'd1', 'd3']);
  });
});
