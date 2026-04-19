import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getFollowings: vi.fn(),
  },
}));

import Following from '@/views/Following.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

const renderFollowing = ({ id = 'bob' } = {}) =>
  render(Following, {
    global: {
      mocks: {
        $router: { push: routerPush },
        $route: { params: { id } },
      },
      stubs: {
        SideNav: true,
        SearchBar: true,
        Loader: true,
        Users: {
          props: ['users', 'loading'],
          template:
            '<ul data-testid="user-list"><li v-for="u in users" :key="u.id">{{ u.username }}</li><li v-if="!loading && users.length === 0">no-users</li></ul>',
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
  routerPush.mockClear();
  warpnetService.getProfile.mockImplementation(async (id) => ({
    id,
    username: id,
  }));
  warpnetService.getFollowings.mockResolvedValue([]);
});

describe('Following.vue', () => {
  it('renders the profile header from the loaded profile', async () => {
    renderFollowing({ id: 'bob' });

    expect(await screen.findByRole('heading', { name: 'bob' })).toBeInTheDocument();
    expect(screen.getByText('@bob')).toBeInTheDocument();
  });

  it('renders the list of users the profile is following', async () => {
    warpnetService.getFollowings.mockResolvedValueOnce(['carol', 'dave']);

    renderFollowing();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(await screen.findByText('dave')).toBeInTheDocument();
  });

  it('shows the empty state when the user follows no one', async () => {
    renderFollowing();

    expect(await screen.findByText('no-users')).toBeInTheDocument();
  });

  it('navigates back to the profile when the back button is clicked', async () => {
    renderFollowing({ id: 'bob' });
    await screen.findByRole('heading', { name: 'bob' });

    const backButton = screen.getAllByRole('button').find((btn) =>
      btn.querySelector('.fa-arrow-left')
    );
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Profile',
        params: { id: 'bob' },
      });
    });
  });

  it('navigates to the Followers tab when the Followers button is clicked', async () => {
    renderFollowing({ id: 'bob' });
    await screen.findByRole('heading', { name: 'bob' });

    await fireEvent.click(screen.getByRole('button', { name: 'Followers' }));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Followers',
        params: { id: 'bob' },
      });
    });
  });

  it('skips followings whose profile cannot be resolved (edge case)', async () => {
    warpnetService.getFollowings.mockResolvedValueOnce(['carol', 'ghost']);
    warpnetService.getProfile.mockImplementation(async (id) => {
      if (id === 'ghost') return null;
      return { id, username: id };
    });

    renderFollowing();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(screen.queryByText('ghost')).not.toBeInTheDocument();
  });
});
