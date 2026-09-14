/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/vue';

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

const push = vi.fn();

const openAccountMenu = async () => {
  render(SideNav, {
    global: {
      mocks: { $router: { push }, $route: { name: 'Home', params: {}, query: {} } },
      stubs: { QRCodeModal: true },
    },
  });
  await fireEvent.click(document.querySelector('[aria-expanded]'));
};

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
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'owner1', username: 'Owner', network: 'testnet' });
  warpnetService.getProfile.mockResolvedValue({ id: 'owner1', username: 'Owner' });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.getNotifications.mockResolvedValue({ notifications: [], unread_count: 0 });
  warpnetService.getWalletHistory.mockResolvedValue([]);
  warpnetService.getQR.mockReturnValue('');
  warpnetService.getQRPayload.mockReturnValue('');
  warpnetService.subscribeNotifications.mockReturnValue(() => {});
  warpnetService.subscribeOwner.mockReturnValue(() => {});
  warpnetService.markMessageNotificationsRead.mockResolvedValue(undefined);
});

describe('SideNav sign out', () => {
  // Stopping the node takes seconds. Without feedback the menu just sits there
  // and the click reads as lost.
  it('answers the click with a spinner and holds it until the node is down', async () => {
    let nodeIsDown;
    warpnetService.logoutUser.mockReturnValue(new Promise((resolve) => { nodeIsDown = resolve; }));

    await openAccountMenu();
    await fireEvent.click(screen.getByText('Sign out'));

    const button = await waitFor(() => screen.getByText('Signing out…').closest('button'));
    expect(button.querySelector('.fa-spinner')).toBeTruthy();
    expect(button.disabled).toBe(true);
    expect(push).not.toHaveBeenCalled();

    nodeIsDown();
    await waitFor(() => expect(push).toHaveBeenCalledWith({ name: 'Root' }));
    expect(document.querySelector('.fa-spinner')).toBeNull();
  });

  it('does not log out twice when the waiting button is clicked again', async () => {
    warpnetService.logoutUser.mockReturnValue(new Promise(() => {}));

    await openAccountMenu();
    await fireEvent.click(screen.getByText('Sign out'));

    const button = await waitFor(() => screen.getByText('Signing out…').closest('button'));
    await fireEvent.click(button);

    expect(warpnetService.logoutUser).toHaveBeenCalledTimes(1);
  });
});
