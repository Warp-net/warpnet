/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getNotifications: vi.fn(),
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
const routerPush = vi.fn();

// Renders SideNav as if the user were already sitting on `routeName`, then
// clicks a nav button by its label. Both the desktop rail and the mobile bar
// render one, and both call open() — clicking the first covers either.
const clickNav = async (routeName, label, params = {}) => {
  render(SideNav, {
    global: {
      mocks: {
        $router: { push: routerPush },
        $route: { name: routeName, params, query: {} },
      },
      stubs: { QRCodeModal: true },
    },
  });
  const button = (await screen.findAllByLabelText(label))[0];
  await fireEvent.click(button);
  return button;
};

let warnSpy, errSpy;
beforeAll(() => {
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  // jsdom ships no matchMedia, and SideNav's mounted() reads it to pick a theme.
  window.matchMedia = vi.fn().mockReturnValue({ matches: false });
});
afterAll(() => {
  warnSpy.mockRestore();
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  routerPush.mockClear();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: OWNER_ID, username: 'Owner' });
  warpnetService.getProfile.mockResolvedValue({ id: OWNER_ID, username: 'Owner' });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.getNotifications.mockResolvedValue({ notifications: [], unread_count: 0 });
  warpnetService.getQR.mockReturnValue('');
  warpnetService.getQRPayload.mockReturnValue('');
  warpnetService.subscribeNotifications.mockReturnValue(() => {});
  warpnetService.subscribeOwner.mockReturnValue(() => {});
  warpnetService.markMessageNotificationsRead.mockResolvedValue(undefined);
});

// Vue Router treats a push to the route you are already on as a no-op, so the
// Home button used to do nothing at all: the timeline kept whatever counters it
// had rendered minutes earlier and there was no way to ask for fresh ones.
describe('SideNav Home button', () => {
  it('navigates normally when Home is not the current route', async () => {
    await clickNav('Notifications', 'Home');

    expect(routerPush).toHaveBeenCalledTimes(1);
    const arg = routerPush.mock.calls[0][0];
    expect(arg.name).toBe('Home');
    expect(arg.query).toBeUndefined();
  });

  it('stamps a refresh query when Home is already the current route', async () => {
    await clickNav('Home', 'Home');

    expect(routerPush).toHaveBeenCalledTimes(1);
    const arg = routerPush.mock.calls[0][0];
    expect(arg.name).toBe('Home');
    expect(arg.query.refresh).toEqual(expect.any(String));
    expect(Number(arg.query.refresh)).toBeGreaterThan(0);
  });

  it('gives every click a distinct stamp so the watcher fires again', async () => {
    const nowSpy = vi.spyOn(Date, 'now');
    nowSpy.mockReturnValueOnce(1700000000000).mockReturnValueOnce(1700000000001);

    const button = await clickNav('Home', 'Home');
    await fireEvent.click(button);

    expect(routerPush).toHaveBeenCalledTimes(2);
    expect(routerPush.mock.calls[0][0].query.refresh)
      .not.toBe(routerPush.mock.calls[1][0].query.refresh);
    nowSpy.mockRestore();
  });

  it('stays a no-op for the other routes open() drives', async () => {
    await clickNav('Notifications', 'Notifications', { id: OWNER_ID });

    expect(routerPush).not.toHaveBeenCalled();
  });

  // The regression that made the button dead in the first place: /home has no
  // :id, so an owner-id comparison always read as "a different view" and the
  // push it produced resolved to the very same URL, which the router drops.
  it('still re-routes to the owner when the current route carries a foreign id', async () => {
    await clickNav('Profile', 'Profile', { id: 'someone-else' });

    expect(routerPush).toHaveBeenCalledTimes(1);
    const arg = routerPush.mock.calls[0][0];
    expect(arg.name).toBe('Profile');
    expect(arg.params.id).toBe(OWNER_ID);
    expect(arg.query).toBeUndefined();
  });
});
