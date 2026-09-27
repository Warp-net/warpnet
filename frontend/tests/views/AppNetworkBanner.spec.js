/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    subscribeOwner: vi.fn(),
    getPendingUpdate: vi.fn(),
  },
}));

vi.mock('@/lib/transport', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

import App from '@/App.vue';
import { warpnetService } from '@/service/service';

const loginScreen = { fullPath: '/', meta: {} };
const home = { fullPath: '/home', meta: { protected: true } };

const mountApp = (route) =>
  render(App, {
    global: {
      stubs: { 'router-view': true, ToastHost: true },
      mocks: { $route: route },
    },
  });

class NoopResizeObserver {
  observe() {}
  disconnect() {}
}

beforeEach(() => {
  vi.clearAllMocks();
  global.ResizeObserver = NoopResizeObserver;
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'u1', network: 'testnet' });
  warpnetService.subscribeOwner.mockReturnValue(() => {});
  warpnetService.getPendingUpdate.mockResolvedValue(null);
});

describe('network banner', () => {
  it('stays off the login screen even when the restored session came from testnet', async () => {
    mountApp(loginScreen);
    await waitFor(() => expect(warpnetService.getPendingUpdate).toHaveBeenCalled());
    expect(screen.queryByText(/data here is experimental/)).toBeNull();
  });

  it('marks a signed-in view of an experimental network', async () => {
    mountApp(home);
    expect(await screen.findByText('Testnet — data here is experimental and may be reset')).toBeTruthy();
  });

  it('stays hidden on the production network', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'u1', network: 'warpnet' });
    mountApp(home);
    await waitFor(() => expect(warpnetService.getPendingUpdate).toHaveBeenCalled());
    expect(screen.queryByText(/data here is experimental/)).toBeNull();
  });
});
