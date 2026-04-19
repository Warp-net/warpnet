import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getFollowers: vi.fn(),
  },
}));

import Followers from '@/views/Followers.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

const renderFollowers = ({ id = 'bob' } = {}) =>
  render(Followers, {
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
  warpnetService.getFollowers.mockResolvedValue([]);
});

describe('Followers.vue', () => {
  it('renders the profile header from the loaded profile', async () => {
    renderFollowers({ id: 'bob' });

    expect(await screen.findByRole('heading', { name: 'bob' })).toBeInTheDocument();
    expect(screen.getByText('@bob')).toBeInTheDocument();
  });

  it('renders the list of followers returned by the service', async () => {
    warpnetService.getFollowers.mockResolvedValueOnce(['carol', 'dave']);

    renderFollowers();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(await screen.findByText('dave')).toBeInTheDocument();
  });

  it('shows the empty state when the user has no followers', async () => {
    renderFollowers();

    expect(await screen.findByText('no-users')).toBeInTheDocument();
  });

  it('navigates back to the profile when the back button is clicked', async () => {
    renderFollowers({ id: 'bob' });
    await screen.findByRole('heading', { name: 'bob' });

    const backButton = screen.getAllByRole('button').find(
      (btn) => btn.querySelector('.fa-arrow-left')
    );
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Profile',
        params: { id: 'bob' },
      });
    });
  });

  it('navigates to the Following tab when the Following button is clicked', async () => {
    renderFollowers({ id: 'bob' });
    await screen.findByRole('heading', { name: 'bob' });

    await fireEvent.click(screen.getByRole('button', { name: 'Following' }));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Following',
        params: { id: 'bob' },
      });
    });
  });

  it('skips followers whose profile cannot be resolved (edge case)', async () => {
    warpnetService.getFollowers.mockResolvedValueOnce(['carol', 'ghost']);
    warpnetService.getProfile.mockImplementation(async (id) => {
      if (id === 'ghost') return null;
      return { id, username: id };
    });

    renderFollowers();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(screen.queryByText('ghost')).not.toBeInTheDocument();
  });
});
