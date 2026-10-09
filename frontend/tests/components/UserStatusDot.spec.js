import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render } from '@testing-library/vue';

vi.mock('@/service/service', () => ({ warpnetService: { getUserStatus: vi.fn() } }));

import UserStatusDot from '@/components/UserStatusDot.vue';
import { warpnetService } from '@/service/service';

const dot = (container) => container.querySelector('[data-testid="user-status"]');

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
});
afterEach(() => {
  vi.useRealTimers();
});

describe('UserStatusDot', () => {
  it('lights green for a user whose node answers', async () => {
    warpnetService.getUserStatus.mockResolvedValue({ user_id: 'd1', is_online: true });
    const { container, unmount } = render(UserStatusDot, { props: { user: { id: 'd1' } } });

    expect(dot(container)).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    expect(dot(container).className).toContain('bg-green-500');
    expect(dot(container).getAttribute('title')).toBe('Online');
    unmount();
  });

  it('lights red with the last time the user was seen', async () => {
    vi.setSystemTime(new Date(2026, 9, 9, 14, 0));
    warpnetService.getUserStatus.mockResolvedValue({
      user_id: 'd2', is_online: false, last_seen: new Date(2026, 9, 9, 12, 5).toISOString(),
    });
    const { container, unmount } = render(UserStatusDot, { props: { user: { id: 'd2' } } });

    await vi.advanceTimersByTimeAsync(0);
    expect(dot(container).className).toContain('bg-red-600');
    expect(dot(container).getAttribute('title')).toBe('Offline · last seen 12:05');
    unmount();
  });

  it('shows nothing while the status is unknown', async () => {
    warpnetService.getUserStatus.mockResolvedValue({});
    const { container, unmount } = render(UserStatusDot, { props: { user: { id: 'd3' } } });

    await vi.advanceTimersByTimeAsync(0);
    expect(dot(container)).toBeNull();
    unmount();
  });

  it('never asks about a bridged account', async () => {
    const { container, unmount } = render(UserStatusDot, {
      props: { user: { id: 'Gargron@mastodon.social', network: 'mastodon' } },
    });

    await vi.advanceTimersByTimeAsync(0);
    expect(warpnetService.getUserStatus).not.toHaveBeenCalled();
    expect(dot(container)).toBeNull();
    unmount();
  });

  it('follows the user when the slot is reused for someone else', async () => {
    warpnetService.getUserStatus.mockImplementation(async (id) => ({ user_id: id, is_online: id === 'd4' }));
    const { container, rerender, unmount } = render(UserStatusDot, { props: { user: { id: 'd4' } } });
    await vi.advanceTimersByTimeAsync(0);
    expect(dot(container).className).toContain('bg-green-500');

    await rerender({ user: { id: 'd5' } });
    await vi.advanceTimersByTimeAsync(0);
    expect(warpnetService.getUserStatus).toHaveBeenLastCalledWith('d5');
    expect(dot(container).className).toContain('bg-red-600');
    unmount();
  });
});
