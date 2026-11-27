/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
  },
}));

import router from '@/router';
import { warpnetService } from '@/service/service';

describe('router /wallet entry', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'alice', network: 'testnet', wallet: false });
    await router.push('/settings');
  });

  it('opens the wallet once it is turned on', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'alice', network: 'testnet', wallet: true });
    await router.push('/wallet');
    expect(router.currentRoute.value.name).toBe('Wallet');
  });

  it('leads to the switch in Settings while the wallet is off', async () => {
    await router.push('/wallet');
    expect(router.currentRoute.value.name).toBe('SettingsWallet');
  });

  it('stays out of reach off testnet even when turned on', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'alice', network: 'warpnet', wallet: true });
    await router.push('/wallet');
    expect(router.currentRoute.value.name).toBe('Root');
  });
});
