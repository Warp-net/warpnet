import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getImage: vi.fn(),
    isFollowing: vi.fn(),
    isUserBlocked: vi.fn(),
    isUserMuted: vi.fn(),
  },
}));

import User from '@/components/User.vue';
import { warpnetService } from '@/service/service';

const renderUser = (user) =>
  render(User, {
    props: { user },
    global: {
      mocks: { $router: { push: vi.fn() } },
      stubs: { ConfirmDialog: true },
    },
  });

let logSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'owner' });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.isFollowing.mockResolvedValue(false);
  warpnetService.isUserBlocked.mockResolvedValue(false);
  warpnetService.isUserMuted.mockResolvedValue(false);
});

describe('User.vue bio', () => {
  it('decodes the entities a bridged bio arrives with', async () => {
    renderUser({
      id: 'eff@mastodon.social',
      username: 'EFF',
      network: 'mastodon',
      bio: 'We&#39;re the EFF &amp; friends',
    });

    expect(await screen.findByText("We're the EFF & friends")).toBeInTheDocument();
  });

  it('leaves a native bio exactly as typed', async () => {
    renderUser({ id: '01ARZ3NDEKTSV4RRFFQ69G5FAV', username: 'Alice', bio: 'Tom &amp; Jerry' });

    expect(await screen.findByText('Tom &amp; Jerry')).toBeInTheDocument();
  });
});
