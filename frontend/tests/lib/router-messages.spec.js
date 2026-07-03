import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
  },
}));

import router from '@/router';
import { warpnetService } from '@/service/service';

describe('router /messages entry', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('redirects /messages to the owner chat list when signed in', async () => {
    warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'alice' });

    await router.push('/messages');

    expect(router.currentRoute.value.name).toBe('Chats');
    expect(router.currentRoute.value.params.id).toBe('alice');
    expect(router.currentRoute.value.path).toBe('/alice/chat');
  });

  it('redirects /messages to the root when signed out', async () => {
    warpnetService.getOwnerProfile.mockReturnValue(undefined);

    await router.push('/messages');

    expect(router.currentRoute.value.name).toBe('Root');
  });
});
