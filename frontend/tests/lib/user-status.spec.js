import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('@/service/service', () => ({ warpnetService: { getUserStatus: vi.fn() } }));

import { userStatus, watchUserStatus } from '@/lib/user-status';
import { warpnetService } from '@/service/service';

const online = (id) => ({ user_id: id, is_online: true, last_seen: '2026-10-09T12:00:00Z' });
const offline = (id) => ({ user_id: id, is_online: false, last_seen: '2026-10-09T11:00:00Z' });

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.spyOn(console, 'warn').mockImplementation(() => {});
});
afterEach(() => {
  vi.useRealTimers();
});

describe('user status', () => {
  it('knows nothing until the node answers, then keeps asking every 30 s', async () => {
    warpnetService.getUserStatus.mockResolvedValueOnce(online('u1')).mockResolvedValueOnce(offline('u1'));

    const unwatch = watchUserStatus('u1');
    expect(userStatus('u1')).toBeUndefined();
    await vi.advanceTimersByTimeAsync(0);
    expect(userStatus('u1')).toEqual({ online: true, lastSeen: '2026-10-09T12:00:00Z' });

    await vi.advanceTimersByTimeAsync(30000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(2);
    expect(userStatus('u1')).toEqual({ online: false, lastSeen: '2026-10-09T11:00:00Z' });
    unwatch();
  });

  it('asks once per user however many places show them', async () => {
    warpnetService.getUserStatus.mockResolvedValue(online('u2'));

    const first = watchUserStatus('u2');
    const second = watchUserStatus('u2');
    await vi.advanceTimersByTimeAsync(0);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(1);
    expect(warpnetService.getUserStatus).toHaveBeenCalledWith('u2');

    first();
    await vi.advanceTimersByTimeAsync(30000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(2);
    second();
  });

  it('stops asking when nobody shows the user', async () => {
    warpnetService.getUserStatus.mockResolvedValue(online('u3'));

    const unwatch = watchUserStatus('u3');
    await vi.advanceTimersByTimeAsync(0);
    unwatch();
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(1);
  });

  it('drops a status as soon as an answer is unclear', async () => {
    warpnetService.getUserStatus
      .mockResolvedValueOnce(online('u4'))
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce(offline('u4'))
      .mockRejectedValueOnce(new Error('ERR_TIMEOUT'));

    const unwatch = watchUserStatus('u4');
    await vi.advanceTimersByTimeAsync(0);
    expect(userStatus('u4').online).toBe(true);
    await vi.advanceTimersByTimeAsync(30000);
    expect(userStatus('u4')).toBeUndefined();
    await vi.advanceTimersByTimeAsync(30000);
    expect(userStatus('u4').online).toBe(false);
    await vi.advanceTimersByTimeAsync(30000);
    expect(userStatus('u4')).toBeUndefined();
    unwatch();
  });

  it('shows a fresh status again at once but not an old one', async () => {
    warpnetService.getUserStatus.mockResolvedValue(online('u5'));

    let unwatch = watchUserStatus('u5');
    await vi.advanceTimersByTimeAsync(0);
    unwatch();

    await vi.advanceTimersByTimeAsync(10000);
    unwatch = watchUserStatus('u5');
    expect(userStatus('u5').online).toBe(true);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(20000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(2);
    unwatch();

    await vi.advanceTimersByTimeAsync(60000);
    unwatch = watchUserStatus('u5');
    expect(userStatus('u5')).toBeUndefined();
    await vi.advanceTimersByTimeAsync(0);
    expect(userStatus('u5').online).toBe(true);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(3);
    unwatch();
  });

  it('does not start a second check while one is still waiting for the node', async () => {
    let answer;
    warpnetService.getUserStatus.mockImplementationOnce(() => new Promise((resolve) => { answer = resolve; }));
    warpnetService.getUserStatus.mockResolvedValue(offline('u6'));

    let unwatch = watchUserStatus('u6');
    await vi.advanceTimersByTimeAsync(0);
    unwatch();
    await vi.advanceTimersByTimeAsync(60000);
    unwatch = watchUserStatus('u6');
    await vi.advanceTimersByTimeAsync(0);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(1);

    answer(online('u6'));
    await vi.advanceTimersByTimeAsync(0);
    expect(userStatus('u6').online).toBe(true);
    await vi.advanceTimersByTimeAsync(30000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(2);
    expect(userStatus('u6').online).toBe(false);
    unwatch();
  });

  it('hides an old status even while a check is still running', async () => {
    let answer;
    warpnetService.getUserStatus.mockResolvedValueOnce(online('u7'));
    warpnetService.getUserStatus.mockImplementationOnce(() => new Promise((resolve) => { answer = resolve; }));

    let unwatch = watchUserStatus('u7');
    await vi.advanceTimersByTimeAsync(30000);
    expect(warpnetService.getUserStatus).toHaveBeenCalledTimes(2);
    unwatch();

    await vi.advanceTimersByTimeAsync(20000);
    unwatch = watchUserStatus('u7');
    expect(userStatus('u7')).toBeUndefined();

    answer(offline('u7'));
    await vi.advanceTimersByTimeAsync(0);
    expect(userStatus('u7').online).toBe(false);
    unwatch();
  });
});
