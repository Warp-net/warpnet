/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    subscribeOwner: vi.fn(),
    consumePendingDeepLink: vi.fn(),
    getPendingUpdate: vi.fn(),
    answerUpdate: vi.fn(),
  },
}));

vi.mock('@/lib/transport', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

import App from '@/App.vue';
import { warpnetService } from '@/service/service';

const pending = { current_version: '0.7.1', new_version: '0.7.2' };

const mountApp = () =>
  render(App, {
    global: {
      stubs: { 'router-view': true, ToastHost: true },
      mocks: { $route: { fullPath: '/' } },
    },
  });

// jsdom has no ResizeObserver, and App.vue measures its banner stack with one.
class NoopResizeObserver {
  observe() {}
  disconnect() {}
}

beforeEach(() => {
  vi.clearAllMocks();
  global.ResizeObserver = NoopResizeObserver;
  warpnetService.getOwnerProfile.mockReturnValue(undefined);
  warpnetService.subscribeOwner.mockReturnValue(() => {});
  warpnetService.getPendingUpdate.mockResolvedValue(null);
  warpnetService.answerUpdate.mockResolvedValue(undefined);
});

describe('update prompt', () => {
  it('stays hidden while the node holds nothing back', async () => {
    mountApp();
    await waitFor(() => expect(warpnetService.getPendingUpdate).toHaveBeenCalled());
    expect(screen.queryByText(/Update available/)).toBeNull();
  });

  it('asks before the node replaces its binary, and reports the answer', async () => {
    warpnetService.getPendingUpdate.mockResolvedValue(pending);
    mountApp();

    await screen.findByText('Update available');
    expect(screen.getByText(/Warpnet 0\.7\.2 is out/)).toBeTruthy();
    expect(screen.getByText(/you are on 0\.7\.1/)).toBeTruthy();

    await fireEvent.click(screen.getByText('Update now'));
    expect(warpnetService.answerUpdate).toHaveBeenCalledWith(true);
    await waitFor(() => expect(screen.queryByText('Update available')).toBeNull());
  });

  it('turns the release down without installing anything', async () => {
    warpnetService.getPendingUpdate.mockResolvedValue(pending);
    mountApp();

    await screen.findByText('Update available');
    await fireEvent.click(screen.getByText('Later'));

    expect(warpnetService.answerUpdate).toHaveBeenCalledWith(false);
    await waitFor(() => expect(screen.queryByText('Update available')).toBeNull());
  });
});
