/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: { logoutUser: vi.fn() },
}));

vi.mock('@/lib/screen-lock', async () => {
  const { reactive } = await import('vue');
  return {
    screenLock: reactive({ isEnabled: true, isLocked: true, idleMinutes: 0, attemptsLeft: 5 }),
    unlockScreen: vi.fn(),
  };
});

import LockScreen from '@/components/LockScreen.vue';
import { warpnetService } from '@/service/service';
import { screenLock as lockState, unlockScreen } from '@/lib/screen-lock';

const push = vi.fn();
const renderLock = () => render(LockScreen, { global: { mocks: { $router: { push } } } });

const enterPin = async (utils, pin) => {
  await fireEvent.update(utils.getByLabelText('PIN'), pin);
  await fireEvent.submit(utils.container.querySelector('form'));
};

let errSpy;
beforeAll(() => {
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterAll(() => {
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  lockState.attemptsLeft = 5;
  warpnetService.logoutUser.mockResolvedValue(undefined);
});

describe('LockScreen', () => {
  it('does not check an empty PIN', async () => {
    const utils = renderLock();
    expect(utils.getByRole('button', { name: 'Unlock' })).toBeDisabled();
    await fireEvent.submit(utils.container.querySelector('form'));
    expect(unlockScreen).not.toHaveBeenCalled();
  });

  it('reports a wrong PIN with the attempts left and stays signed in', async () => {
    unlockScreen.mockImplementation(async () => {
      lockState.attemptsLeft -= 1;
      return false;
    });
    const utils = renderLock();
    await enterPin(utils, '0000');

    await waitFor(() => expect(utils.getByRole('alert')).toHaveTextContent('4 attempts left'));
    expect(utils.getByLabelText('PIN')).toHaveValue('');
    expect(warpnetService.logoutUser).not.toHaveBeenCalled();
    expect(push).not.toHaveBeenCalled();
  });

  it('says "1 attempt" on the last try', async () => {
    lockState.attemptsLeft = 2;
    unlockScreen.mockImplementation(async () => {
      lockState.attemptsLeft -= 1;
      return false;
    });
    const utils = renderLock();
    await enterPin(utils, '0000');
    await waitFor(() => expect(utils.getByRole('alert')).toHaveTextContent('1 attempt left'));
  });

  it('signs out after the last wrong PIN', async () => {
    lockState.attemptsLeft = 1;
    unlockScreen.mockImplementation(async () => {
      lockState.attemptsLeft = 0;
      return false;
    });
    const utils = renderLock();
    await enterPin(utils, '0000');

    await waitFor(() => expect(push).toHaveBeenCalledWith({ name: 'Root' }));
    expect(warpnetService.logoutUser).toHaveBeenCalledTimes(1);
  });

  it('signs out at once when it opens with no attempts left', async () => {
    lockState.attemptsLeft = 0;
    renderLock();
    await waitFor(() => expect(push).toHaveBeenCalledWith({ name: 'Root' }));
    expect(warpnetService.logoutUser).toHaveBeenCalledTimes(1);
    expect(unlockScreen).not.toHaveBeenCalled();
  });

  it('still leaves for the login screen when sign-out fails', async () => {
    warpnetService.logoutUser.mockRejectedValue(new Error('node unreachable'));
    const utils = renderLock();
    await fireEvent.click(utils.getByRole('button', { name: /Sign out/ }));
    await waitFor(() => expect(push).toHaveBeenCalledWith({ name: 'Root' }));
  });
});
