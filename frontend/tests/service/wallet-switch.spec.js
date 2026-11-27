/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_POST_SETTINGS_WALLET } from '@/service/service';
import { Call } from '@/lib/transport';

const owner = { user_id: 'owner1', node_id: 'node-owner', network: 'testnet' };

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  warpnetService.setOwnerProfile({ ...owner });
});

describe('wallet switch', () => {
  it('sends the switch with the settings and keeps it on the owner', async () => {
    Call.mockResolvedValue({ body: { enabled: true, endpoint: '', binary_path: '', headers: {} } });

    await warpnetService.updateWalletSettings({ enabled: true, endpoint: ' https://nile.example ' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_SETTINGS_WALLET);
    expect(sent.body).toEqual({ enabled: true, endpoint: 'https://nile.example', binary_path: '', headers: {} });
    expect(warpnetService.getOwnerProfile().wallet).toBe(true);
    expect(JSON.parse(localStorage.getItem('warpnet.owner')).wallet).toBe(true);
  });

  it('sends the wallet off when the switch is not given', async () => {
    warpnetService.setOwnerProfile({ ...owner, wallet: true });
    Call.mockResolvedValue({ body: { enabled: false, endpoint: '', binary_path: '', headers: {} } });

    await warpnetService.updateWalletSettings({ endpoint: '' });

    expect(Call.mock.calls[0][0].body.enabled).toBe(false);
    expect(warpnetService.getOwnerProfile().wallet).toBe(false);
  });

  it('learns the switch from the node when the settings are read', async () => {
    Call.mockResolvedValue({ body: { enabled: true, endpoint: 'https://nile.example' } });

    await warpnetService.getWalletSettings();

    expect(warpnetService.getOwnerProfile()).toEqual({ ...owner, wallet: true });
  });

  it('leaves the owner as it was when the node refuses to answer', async () => {
    Call.mockResolvedValue({ body: { code: 500, message: 'not attached server node' } });

    await warpnetService.getWalletSettings();

    expect(warpnetService.getOwnerProfile()).toEqual(owner);
  });
});
