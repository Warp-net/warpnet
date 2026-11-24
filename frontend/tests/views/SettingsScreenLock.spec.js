/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(() => ({ user_id: 'u1' })),
    logoutUser: vi.fn(),
  },
}));

vi.mock('@/lib/screen-lock', async () => {
  const { reactive } = await import('vue');
  return {
    IDLE_MINUTES_CHOICES: [0, 1, 5, 15, 30, 60],
    MAX_UNLOCK_ATTEMPTS: 5,
    PIN_PATTERN: /^\d{4,8}$/,
    screenLock: reactive({ isEnabled: false, isLocked: false, idleMinutes: 0, attemptsLeft: 5 }),
    updateScreenLock: vi.fn(),
    removeScreenLock: vi.fn(),
  };
});

import ScreenLock from '@/views/Settings/ScreenLock.vue';
import { warpnetService } from '@/service/service';
import { removeScreenLock, screenLock as lockState, updateScreenLock } from '@/lib/screen-lock';

const push = vi.fn();
const renderPage = () =>
  render(ScreenLock, {
    global: {
      mocks: { $router: { push } },
      stubs: { SideNav: true, DefaultRightBar: true },
    },
  });

const passwordInputs = (container) => [...container.querySelectorAll('input[type=password]')];

beforeEach(() => {
  vi.clearAllMocks();
  Object.assign(lockState, { isEnabled: false, isLocked: false, idleMinutes: 0, attemptsLeft: 5 });
  warpnetService.logoutUser.mockResolvedValue(undefined);
});

describe('Screen lock settings', () => {
  it('will not turn on without a PIN', async () => {
    const utils = renderPage();
    await fireEvent.click(utils.getByRole('button', { name: 'Turn on' }));
    await waitFor(() => expect(utils.getByRole('status')).toHaveTextContent('Choose a PIN first.'));
    expect(updateScreenLock).not.toHaveBeenCalled();
  });

  it('rejects a PIN that is not 4 to 8 digits', async () => {
    const utils = renderPage();
    await fireEvent.update(passwordInputs(utils.container)[0], '12ab');
    await fireEvent.click(utils.getByRole('button', { name: 'Turn on' }));
    await waitFor(() => expect(utils.getByRole('status')).toHaveTextContent('4 to 8 digits'));
    expect(updateScreenLock).not.toHaveBeenCalled();
  });

  it('rejects a repeated PIN that does not match', async () => {
    const utils = renderPage();
    await fireEvent.update(passwordInputs(utils.container)[0], '1234');
    await fireEvent.update(passwordInputs(utils.container)[1], '1243');
    await fireEvent.click(utils.getByRole('button', { name: 'Turn on' }));
    await waitFor(() => expect(utils.getByRole('status')).toHaveTextContent('do not match'));
    expect(updateScreenLock).not.toHaveBeenCalled();
  });

  it('reports a wrong current PIN and keeps the user signed in', async () => {
    Object.assign(lockState, { isEnabled: true, idleMinutes: 5 });
    updateScreenLock.mockImplementation(async () => {
      lockState.attemptsLeft = 3;
      return false;
    });
    const utils = renderPage();
    await fireEvent.update(passwordInputs(utils.container)[0], '0000');
    await fireEvent.click(utils.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(utils.getByRole('status')).toHaveTextContent('3 attempts left'));
    expect(passwordInputs(utils.container)[0]).toHaveValue('');
    expect(warpnetService.logoutUser).not.toHaveBeenCalled();
  });

  it('signs out when turning off spends the last attempt', async () => {
    Object.assign(lockState, { isEnabled: true, attemptsLeft: 1 });
    removeScreenLock.mockImplementation(async () => {
      lockState.attemptsLeft = 0;
      return false;
    });
    const utils = renderPage();
    await fireEvent.update(passwordInputs(utils.container)[0], '0000');
    await fireEvent.click(utils.getByRole('button', { name: 'Turn off' }));

    await waitFor(() => expect(push).toHaveBeenCalledWith({ name: 'Root' }));
    expect(warpnetService.logoutUser).toHaveBeenCalledTimes(1);
  });
});
