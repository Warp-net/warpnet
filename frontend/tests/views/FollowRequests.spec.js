import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getFollowRequests: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    authorizeFollowRequest: vi.fn(),
    rejectFollowRequest: vi.fn(),
  },
}));

import FollowRequests from '@/views/FollowRequests.vue';
import { warpnetService } from '@/service/service';

const renderFollowRequests = () =>
  render(FollowRequests, {
    global: {
      mocks: {
        $router: { push: vi.fn() },
      },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: {
          props: ['loading'],
          template: '<div v-if="loading" data-testid="loader" />',
        },
      },
    },
  });

let logSpy, errSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'me' });
  warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: [] });
  warpnetService.getProfile.mockImplementation(async (id) => ({ id, username: id }));
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.authorizeFollowRequest.mockResolvedValue({});
  warpnetService.rejectFollowRequest.mockResolvedValue({});
});

describe('FollowRequests.vue', () => {
  it('shows the empty state when there are no requests', async () => {
    renderFollowRequests();

    expect(await screen.findByText(/No follow requests/i)).toBeInTheDocument();
  });

  it('renders a request row once its profile resolves', async () => {
    warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: ['bob'] });
    warpnetService.getProfile.mockResolvedValue({ id: 'bob', username: 'Bobby' });

    renderFollowRequests();

    expect(await screen.findByText('Bobby')).toBeInTheDocument();
    expect(screen.getByText('@bob')).toBeInTheDocument();
  });

  it('a hanging profile request does not block the first paint', async () => {
    warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: ['bob', 'slow'] });
    warpnetService.getProfile.mockImplementation((id) =>
      id === 'slow' ? new Promise(() => {}) : Promise.resolve({ id, username: id })
    );

    renderFollowRequests();

    // Both rows are on screen immediately: bob resolved, slow as placeholder.
    expect(await screen.findByText('@bob', undefined, { timeout: 3000 })).toBeInTheDocument();
    expect(screen.getByText('@slow')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByTestId('loader')).not.toBeInTheDocument());
  });

  it('removes the row when a request is authorized', async () => {
    warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: ['bob'] });

    renderFollowRequests();
    await screen.findByText('@bob');

    await fireEvent.click(screen.getByRole('button', { name: 'Authorize' }));

    await waitFor(() => {
      expect(warpnetService.authorizeFollowRequest).toHaveBeenCalledWith('bob');
      expect(screen.queryByText('@bob')).not.toBeInTheDocument();
    });
  });
});
