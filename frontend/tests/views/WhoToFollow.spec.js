import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getWhoToFollow: vi.fn(),
  },
}));

import WhoToFollow from '@/views/WhoToFollow.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

const renderView = () =>
  render(WhoToFollow, {
    global: {
      mocks: {
        $router: { push: routerPush },
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
  warpnetService.getWhoToFollow.mockResolvedValue([]);
});

// Valid ULID → warpnet; fediverse handle → mastodon.
const WARPNET_ID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';
const MASTODON_ID = 'bob@mastodon.social';

describe('WhoToFollow.vue', () => {
  it('splits the recommendations into Warpnet and Mastodon sections', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce([
      { id: WARPNET_ID, username: 'alice' },
      { id: MASTODON_ID, username: 'bob' },
    ]);

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(screen.getByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.getByLabelText('Mastodon')).toBeInTheDocument();
    // owner-scoped: reset cursor, no profile id is passed through
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledWith(true);
  });

  it('hides a section when there are no users from that network', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce([
      { id: WARPNET_ID, username: 'alice' },
    ]);

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.getByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.queryByLabelText('Mastodon')).not.toBeInTheDocument();
  });

  it('shows the empty state when there are no recommendations', async () => {
    renderView();

    expect(await screen.findByText('No results')).toBeInTheDocument();
  });

  it('appends the next page when "Show More" is clicked', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([{ id: WARPNET_ID, username: 'alice' }])
      .mockResolvedValueOnce([{ id: MASTODON_ID, username: 'bob' }]);

    renderView();
    await screen.findByText('alice');

    await fireEvent.click(screen.getByRole('button', { name: 'Show More' }));

    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith(false);
  });

  it('hides "Show More" once a page comes back empty (end state)', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce([{ id: WARPNET_ID, username: 'alice' }]);

    renderView();
    await screen.findByText('alice');

    await fireEvent.click(screen.getByRole('button', { name: 'Show More' }));

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument();
    });
  });

  it('navigates home when the back button is clicked', async () => {
    renderView();
    await screen.findByText('No results');

    const backButton = screen
      .getAllByRole('button')
      .find((btn) => btn.querySelector('.fa-arrow-left'));
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith('/');
    });
  });
});
