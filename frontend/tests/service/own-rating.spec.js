/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_GET_RATING } from '@/service/service';
import { Call } from '@/lib/transport';

let errSpy;
beforeAll(() => {
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterAll(() => {
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
});

describe('getOwnRating', () => {
  it('asks the node on the private rating route with an empty body', async () => {
    Call.mockResolvedValue({ body: { overall: 1000, tier: 'trusted', observers: 0, dimensions: null } });

    await warpnetService.getOwnRating();

    expect(Call).toHaveBeenCalledTimes(1);
    const sent = Call.mock.calls[0][0];
    // Mirrors event.PRIVATE_GET_RATING in event/paths.go.
    expect(PRIVATE_GET_RATING).toBe('/private/get/admin/rating/0.0.0');
    expect(sent.path).toBe(PRIVATE_GET_RATING);
    expect(sent.body).toEqual({});
    expect(sent.node_id).toBe('node-owner');
  });

  it('hands back the rating the node answered with', async () => {
    const rating = {
      node_id: 'node-owner',
      overall: 742,
      tier: 'watched',
      dimensions: [
        {
          name: 'net',
          score: 742,
          tier: 'watched',
          recent: [{ kind: 'rate_limit_hit', count: 12, last_at: '2026-09-29T23:00:00Z' }],
        },
      ],
      observers: 3,
      updated_at: '2026-09-30T00:00:00Z',
    };
    Call.mockResolvedValue({ code: 200, body: rating });

    await expect(warpnetService.getOwnRating()).resolves.toEqual(rating);
  });

  // The page reads a missing overall as "not available", so a refusal must
  // never come back carrying one.
  it.each([
    ['an error body', { body: { code: 500, message: 'rating is not available on this node' } }],
    ['no body at all', { code: 500, message: 'rating is not available on this node' }],
  ])('answers without an overall when the node refuses with %s', async (_, answer) => {
    Call.mockResolvedValue(answer);

    const resp = await warpnetService.getOwnRating();

    expect(resp).not.toHaveProperty('overall');
  });

  it('rejects when the transport returns nothing', async () => {
    Call.mockResolvedValue(undefined);

    await expect(warpnetService.getOwnRating()).rejects.toThrow('Unable to send');
  });

  it('passes a transport failure through', async () => {
    Call.mockRejectedValue(new Error('engine unavailable'));

    await expect(warpnetService.getOwnRating()).rejects.toThrow('engine unavailable');
  });
});
