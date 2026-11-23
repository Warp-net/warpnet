import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

let lock;

beforeEach(async () => {
  localStorage.clear();
  vi.resetModules();
  lock = await import('@/lib/screen-lock');
  lock.attachScreenLock('u1');
});
afterEach(() => {
  lock.attachScreenLock('');
  vi.useRealTimers();
});

describe('screen lock', () => {
  it('stays off until a PIN is set', () => {
    lock.lockScreen();
    expect(lock.screenLock.isEnabled).toBe(false);
    expect(lock.screenLock.isLocked).toBe(false);
  });

  it('refuses a PIN that is not 4 to 8 digits', async () => {
    await expect(lock.updateScreenLock({ newPin: '12a4', idleMinutes: 5 })).rejects.toThrow();
    await expect(lock.updateScreenLock({ newPin: '123', idleMinutes: 5 })).rejects.toThrow();
    expect(lock.screenLock.isEnabled).toBe(false);
  });

  it('unlocks only with the right PIN and counts the wrong ones down', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    expect(lock.screenLock.isLocked).toBe(true);

    expect(await lock.unlockScreen('0000')).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS - 1);
    expect(lock.screenLock.isLocked).toBe(true);

    expect(await lock.unlockScreen('1234')).toBe(true);
    expect(lock.screenLock.isLocked).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS);
  });

  it('runs out of attempts after five wrong PINs', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    for (let i = 0; i < lock.MAX_UNLOCK_ATTEMPTS; i++) {
      await lock.unlockScreen('9999');
    }
    expect(lock.screenLock.attemptsLeft).toBe(0);
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('comes back locked, with the same attempts left, after a reload', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    await lock.unlockScreen('0000');

    vi.resetModules();
    const reloaded = await import('@/lib/screen-lock');
    reloaded.attachScreenLock('u1');
    expect(reloaded.screenLock.isLocked).toBe(true);
    expect(reloaded.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS - 1);
    reloaded.attachScreenLock('');
  });

  it('is cleared by a password login', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    await lock.unlockScreen('0000');
    lock.resetScreenLock('u1');
    expect(lock.screenLock.isLocked).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS);
    expect(lock.screenLock.isEnabled).toBe(true);
  });

  it('changes or removes the lock only with the current PIN', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 5 });
    expect(await lock.updateScreenLock({ currentPin: '0000', newPin: '5678', idleMinutes: 5 })).toBe(false);
    expect(await lock.removeScreenLock('0000')).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS - 2);

    expect(await lock.updateScreenLock({ currentPin: '1234', newPin: '5678', idleMinutes: 15 })).toBe(true);
    expect(lock.screenLock.idleMinutes).toBe(15);
    expect(await lock.removeScreenLock('5678')).toBe(true);
    expect(lock.screenLock.isEnabled).toBe(false);
  });

  it('locks after the idle timeout and input postpones it', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 1 });
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    lock.attachScreenLock('u1');

    vi.advanceTimersByTime(45000);
    document.dispatchEvent(new Event('pointerdown'));
    vi.advanceTimersByTime(45000);
    expect(lock.screenLock.isLocked).toBe(false);

    vi.advanceTimersByTime(30000);
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('locks on Ctrl+L', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', ctrlKey: true }));
    expect(lock.screenLock.isLocked).toBe(true);
  });
});
