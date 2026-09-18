/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_GET_WALLET_HISTORY } from '@/service/service';
import { Call } from '@/lib/transport';

const window_ = 60000;
let clock = Date.parse('2026-01-01T00:00:00Z');

beforeEach(() => {
  vi.clearAllMocks();
  clock += 10 * window_;
  vi.useFakeTimers();
  vi.setSystemTime(clock);
  Call.mockResolvedValue({ code: 200, body: { transfers: [{ tx: 'a', incoming: true, timestamp: 1000 }] } });
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
});

afterEach(() => {
  vi.useRealTimers();
});

describe('getWalletHistory', () => {
  it('reads the chain once while the answer is fresh, however many views ask', async () => {
    const first = await warpnetService.getWalletHistory(25, 'TRX');
    const second = await warpnetService.getWalletHistory(25, 'TRX');

    expect(Call).toHaveBeenCalledTimes(1);
    expect(Call.mock.calls[0][0].path).toBe(PRIVATE_GET_WALLET_HISTORY);
    expect(second).toEqual(first);
  });

  it('reads both assets: one held answer does not stand in for the other', async () => {
    await warpnetService.getWalletHistory(25, 'TRX');
    await warpnetService.getWalletHistory(25, 'USDT');

    expect(Call).toHaveBeenCalledTimes(2);
    expect(Call.mock.calls.map((c) => c[0].body.asset)).toEqual(['TRX', 'USDT']);
  });

  it('goes to the chain anyway when the caller forces it', async () => {
    await warpnetService.getWalletHistory(25, 'TRX');
    await warpnetService.getWalletHistory(25, 'TRX', true);

    expect(Call).toHaveBeenCalledTimes(2);
  });

  it('asks again once the held answer is stale', async () => {
    await warpnetService.getWalletHistory(25, 'TRX');
    vi.setSystemTime(clock + window_ + 1);
    await warpnetService.getWalletHistory(25, 'TRX');

    expect(Call).toHaveBeenCalledTimes(2);
  });

  it('does not hold a read that failed', async () => {
    Call.mockRejectedValueOnce(new Error('engine unavailable'));
    await expect(warpnetService.getWalletHistory(25, 'TRX')).rejects.toThrow('engine unavailable');

    expect(await warpnetService.getWalletHistory(25, 'TRX')).toEqual([{ tx: 'a', incoming: true, timestamp: 1000 }]);
    expect(Call).toHaveBeenCalledTimes(2);
  });
});
