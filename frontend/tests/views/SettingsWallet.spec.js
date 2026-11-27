/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getWalletSettings: vi.fn(),
    updateWalletSettings: vi.fn(),
  },
}));
vi.mock('@/lib/toast', () => ({ toast: { error: vi.fn() } }));

import SettingsWallet from '@/views/Settings/Wallet.vue';
import { warpnetService } from '@/service/service';

const renderLoaded = async (saved) => {
  warpnetService.getWalletSettings.mockResolvedValue(saved);
  render(SettingsWallet, {
    global: {
      mocks: { $router: { push: vi.fn() } },
      stubs: { SideNav: true, DefaultRightBar: true, Loader: true },
    },
  });
  return screen.findByTestId('wallet-enabled');
};

describe('Settings/Wallet.vue', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    warpnetService.updateWalletSettings.mockResolvedValue({});
  });

  it('starts with the wallet off and turns it on right away, keeping the backend fields', async () => {
    const box = await renderLoaded({ endpoint: 'https://nile.example', binary_path: '', headers: { 'TRON-PRO-API-KEY': 'k' } });
    expect(box.checked).toBe(false);

    await fireEvent.click(box);

    await waitFor(() => expect(warpnetService.updateWalletSettings).toHaveBeenCalledWith({
      enabled: true, endpoint: 'https://nile.example', binary_path: '', headers: { 'TRON-PRO-API-KEY': 'k' },
    }));
  });

  it('keeps the switch where it was when the node refuses', async () => {
    warpnetService.updateWalletSettings.mockRejectedValue(new Error('db closed'));
    const box = await renderLoaded({ enabled: false, endpoint: '' });

    await fireEvent.click(box);

    await waitFor(() => expect(screen.getByText('Failed to save')).toBeInTheDocument());
    expect(box.checked).toBe(false);
  });

  it('restores the backend defaults without turning the wallet off', async () => {
    await renderLoaded({ enabled: true, endpoint: 'https://nile.example' });

    await fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' }));

    expect(warpnetService.updateWalletSettings).toHaveBeenCalledWith({ enabled: true });
  });
});
