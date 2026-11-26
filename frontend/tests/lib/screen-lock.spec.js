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

    expect(await lock.unlockScreen('1234')).toBe(false);
    expect(await lock.removeScreenLock('1234')).toBe(false);
    expect(await lock.updateScreenLock({ currentPin: '1234', newPin: '5678', idleMinutes: 0 })).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(0);
    expect(lock.screenLock.isLocked).toBe(true);
    expect(lock.screenLock.isEnabled).toBe(true);
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

describe('screen lock refuses', () => {
  it('to unlock when no PIN was ever set, without spending attempts', async () => {
    expect(await lock.unlockScreen('1234')).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS);
    expect(await lock.removeScreenLock('1234')).toBe(false);
  });

  it('an empty or missing PIN and counts it as a wrong one', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    expect(await lock.unlockScreen('')).toBe(false);
    expect(await lock.unlockScreen(undefined)).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS - 2);
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('a PIN that only starts with the right digits', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();
    expect(await lock.unlockScreen('12345')).toBe(false);
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('to turn on without a PIN', async () => {
    await expect(lock.updateScreenLock({ idleMinutes: 5 })).rejects.toThrow();
    await expect(lock.updateScreenLock({ newPin: '123456789', idleMinutes: 5 })).rejects.toThrow();
    await expect(lock.updateScreenLock({ newPin: ' 1234', idleMinutes: 5 })).rejects.toThrow();
    expect(lock.screenLock.isEnabled).toBe(false);
    expect(localStorage.getItem('warpnet.lock.u1')).toBeNull();
  });

  it('an invalid new PIN and keeps the old one', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 5 });
    await expect(lock.updateScreenLock({ currentPin: '1234', newPin: '12', idleMinutes: 15 })).rejects.toThrow();
    expect(lock.screenLock.idleMinutes).toBe(5);
    lock.lockScreen();
    expect(await lock.unlockScreen('1234')).toBe(true);
  });

  it('an unknown idle timeout and falls back to never', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 7 });
    expect(lock.screenLock.idleMinutes).toBe(0);
  });

  it('to drop the lock on a wrong PIN', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    expect(await lock.removeScreenLock('4321')).toBe(false);
    expect(lock.screenLock.isEnabled).toBe(true);
    lock.lockScreen();
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('to lock on keys other than Ctrl/Cmd+L, or with no PIN set', async () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', ctrlKey: true }));
    expect(lock.screenLock.isLocked).toBe(false);

    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'l' }));
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }));
    expect(lock.screenLock.isLocked).toBe(false);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'L', metaKey: true }));
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('to auto-lock when the timeout is never', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    lock.attachScreenLock('u1');
    vi.advanceTimersByTime(2 * 60 * 60 * 1000);
    expect(lock.screenLock.isLocked).toBe(false);
  });

  it('to lock anything once signed out', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 1 });
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] });
    lock.attachScreenLock('');
    vi.advanceTimersByTime(10 * 60 * 1000);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', ctrlKey: true }));
    lock.lockScreen();
    expect(lock.screenLock.isLocked).toBe(false);
    expect(lock.screenLock.isEnabled).toBe(false);
  });

  it("to share one owner's lock with another account", async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    lock.lockScreen();

    lock.attachScreenLock('u2');
    expect(lock.screenLock.isEnabled).toBe(false);
    expect(lock.screenLock.isLocked).toBe(false);

    lock.attachScreenLock('u1');
    lock.resetScreenLock('u2');
    expect(lock.screenLock.isLocked).toBe(true);
  });

  it('to trust corrupted or inconsistent storage', () => {
    localStorage.setItem('warpnet.lock.u1', '{broken');
    expect(() => lock.attachScreenLock('u1')).not.toThrow();
    expect(lock.screenLock.isEnabled).toBe(false);

    localStorage.removeItem('warpnet.lock.u1');
    localStorage.setItem('warpnet.lock.u1.state', JSON.stringify({ isLocked: true, attemptsLeft: 2 }));
    lock.attachScreenLock('u1');
    expect(lock.screenLock.isLocked).toBe(false);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS);
  });

  it('to reset spent attempts through a garbage state', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    localStorage.setItem('warpnet.lock.u1.state', JSON.stringify({ isLocked: true, attemptsLeft: 'x' }));
    lock.attachScreenLock('u1');
    expect(lock.screenLock.isLocked).toBe(true);
    expect(lock.screenLock.attemptsLeft).toBe(lock.MAX_UNLOCK_ATTEMPTS);
  });

  it('to crash when storage is unavailable, still locking in memory', async () => {
    await lock.updateScreenLock({ newPin: '1234', idleMinutes: 0 });
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    try {
      expect(() => lock.lockScreen()).not.toThrow();
      expect(lock.screenLock.isLocked).toBe(true);
    } finally {
      setItem.mockRestore();
    }
  });
});
