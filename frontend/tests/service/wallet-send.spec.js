/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_POST_WALLET_SEND } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
});

describe('sendFunds', () => {
  it('hands back the transaction the node sent', async () => {
    Call.mockResolvedValue({ body: { tx: 'tx-1', to: 'TPeer', amount: '1000000', asset: 'TRX' } });

    const resp = await warpnetService.sendFunds('TPeer', '1000000', 'TRX');

    expect(Call.mock.calls[0][0].path).toBe(PRIVATE_POST_WALLET_SEND);
    expect(resp.tx).toBe('tx-1');
  });

  it('says why when the node refuses to send', async () => {
    Call.mockResolvedValue({ body: { code: 500, message: 'wallet: the recipient has the wallet turned off; nothing was sent' } });

    await expect(warpnetService.sendFunds('TPeer', '1000000', 'TRX'))
      .rejects.toThrow('wallet: the recipient has the wallet turned off; nothing was sent');
  });

  it('does not report a send that has no transaction', async () => {
    Call.mockResolvedValue({ code: 500, message: 'node down' });

    await expect(warpnetService.sendFunds('TPeer', '1000000', 'TRX')).rejects.toThrow();
  });
});
