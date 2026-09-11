/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getNotifications: vi.fn(),
    getWalletHistory: vi.fn(),
    getQR: vi.fn(),
    getQRPayload: vi.fn(),
    logoutUser: vi.fn(),
    markMessageNotificationsRead: vi.fn(),
    subscribeNotifications: vi.fn(),
    subscribeOwner: vi.fn(),
  },
}));

import SideNav from '@/components/SideNav.vue';
import { warpnetService } from '@/service/service';

const OWNER_ID = 'owner1';
const SEEN_KEY = `warpnet:wallet-seen:${OWNER_ID}`;

const renderNav = (routeName = 'Home') =>
  render(SideNav, {
    global: {
      mocks: {
        $router: { push: vi.fn() },
        $route: { name: routeName, params: {}, query: {} },
      },
      stubs: { QRCodeModal: true },
    },
  });

let warnSpy, errSpy;
beforeAll(() => {
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  window.matchMedia = vi.fn().mockReturnValue({ matches: false });
});
afterAll(() => {
  warnSpy.mockRestore();
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: OWNER_ID, username: 'Owner', network: 'testnet' });
  warpnetService.getProfile.mockResolvedValue({ id: OWNER_ID, username: 'Owner' });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.getNotifications.mockResolvedValue({ notifications: [], unread_count: 0 });
  warpnetService.getWalletHistory.mockResolvedValue([]);
  warpnetService.getQR.mockReturnValue('');
  warpnetService.getQRPayload.mockReturnValue('');
  warpnetService.subscribeNotifications.mockReturnValue(() => {});
  warpnetService.subscribeOwner.mockReturnValue(() => {});
  warpnetService.markMessageNotificationsRead.mockResolvedValue(undefined);
});

describe('SideNav wallet badge', () => {
  it('does not badge a wallet that was never opened, it just records where now is', async () => {
    warpnetService.getWalletHistory.mockResolvedValue([
      { tx: 'a', incoming: true, timestamp: 1000 },
      { tx: 'b', incoming: true, timestamp: 900 },
    ]);
    renderNav();
    await waitFor(() => expect(warpnetService.getWalletHistory).toHaveBeenCalled());
    await waitFor(() => expect(localStorage.getItem(SEEN_KEY)).toBe('1000'));
    expect(screen.queryByTestId('wallet-badge')).toBeNull();
  });

  it('counts only the incoming transfers that arrived since the wallet was last opened', async () => {
    localStorage.setItem(SEEN_KEY, '500');
    warpnetService.getWalletHistory.mockResolvedValue([
      { tx: 'old', incoming: true, timestamp: 400 },
      { tx: 'new1', incoming: true, timestamp: 600 },
      { tx: 'new2', incoming: true, timestamp: 700 },
      { tx: 'sent', incoming: false, timestamp: 800 },
    ]);
    renderNav();
    await waitFor(() => expect(screen.getByTestId('wallet-badge').textContent.trim()).toBe('2'));
  });

  it('clears the badge while the wallet tab is the current route', async () => {
    localStorage.setItem(SEEN_KEY, '500');
    warpnetService.getWalletHistory.mockResolvedValue([
      { tx: 'new1', incoming: true, timestamp: 600 },
    ]);
    renderNav('Wallet');
    await waitFor(() => expect(Number(localStorage.getItem(SEEN_KEY))).toBeGreaterThan(500));
    expect(screen.queryByTestId('wallet-badge')).toBeNull();
  });

  it('never asks for wallet history when the wallet is not offered', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: OWNER_ID, username: 'Owner', network: 'mainnet' });
    renderNav();
    await waitFor(() => expect(warpnetService.getNotifications).toHaveBeenCalled());
    expect(warpnetService.getWalletHistory).not.toHaveBeenCalled();
    expect(screen.queryByLabelText('Wallet')).toBeNull();
  });
});
