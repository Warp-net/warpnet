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
    isUnfamiliar: vi.fn((id) => Promise.resolve(id === 'sam')),
    subscribeOwner: vi.fn(),
  },
}));

import SideNav from '@/components/SideNav.vue';
import { warpnetService } from '@/service/service';

const OWNER_ID = 'owner1';

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

describe('SideNav notification badges', () => {
  it('leaves unread items from unfamiliar people out of both badges', async () => {
    let push;
    warpnetService.subscribeNotifications.mockImplementation((cb) => {
      push = cb;
      return () => {};
    });
    renderNav();
    await waitFor(() => expect(push).toBeTypeOf('function'));

    await push({
      unread_count: 5,
      notifications: [
        { id: 'n1', type: 'follow', actor_id: 'bob', is_read: false },
        { id: 'n2', type: 'reply', actor_id: 'sam', is_read: false },
        { id: 'n3', type: 'message', actor_id: 'bob', is_read: false },
        { id: 'n4', type: 'message', actor_id: 'sam', is_read: false },
        { id: 'n5', type: 'order_limit', actor_id: 'sam', is_read: false },
      ],
    });

    const [notifications] = screen.getAllByRole('button', { name: 'Notifications' });
    const [messages] = screen.getAllByRole('button', { name: 'Messages' });
    await waitFor(() => expect(notifications).toHaveTextContent('2'));
    expect(messages).toHaveTextContent('1');
  });
});
