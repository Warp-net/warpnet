import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    isUnfamiliar: vi.fn(() => Promise.resolve(false)),
    acceptChat: vi.fn(),
    circleFirst: vi.fn((users) => Promise.resolve(users || [])),
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
  warpnetService.getWhoToFollow.mockResolvedValue({});
  warpnetService.circleFirst.mockImplementation((users) => Promise.resolve(users || []));
});

// Valid ULID → warpnet; fediverse handle → mastodon.
const WARPNET_ID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';
const MASTODON_ID = 'bob@mastodon.social';
const page = (users, cursor = null) => ({ users, cursor });

describe('WhoToFollow.vue', () => {
  it('splits the recommendations into Warpnet and Mastodon sections', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
    });

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(screen.getByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.getByLabelText('Mastodon')).toBeInTheDocument();
    // owner-scoped first page: no cursors, no profile id is passed through
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledWith();
  });

  it('gives a network the client does not know its own section', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      bluesky: page([{ id: 'carol@bsky.social', username: 'carol', network: 'bluesky' }]),
    });

    renderView();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(screen.getByText('Bluesky')).toBeInTheDocument();
    expect(screen.getByLabelText('bluesky')).toBeInTheDocument();
  });

  it('hides a section when there are no users from that network', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
    });

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.getByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.queryByLabelText('Mastodon')).not.toBeInTheDocument();
  });

  it('shows the empty state when there are no recommendations', async () => {
    renderView();

    expect(await screen.findByText('No results')).toBeInTheDocument();
  });

  it('asks for the next page of every network that has one', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({
        warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
        mastodon: page([], 'm-next'),
      })
      .mockResolvedValueOnce({ mastodon: page([{ id: MASTODON_ID, username: 'bob' }]) });

    renderView();
    await screen.findByText('alice');

    await fireEvent.click(screen.getByRole('button', { name: 'Show More' }));

    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' });
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument();
    });
  });

  it('hides "Show More" when every network has run out', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }]) });

    renderView();
    await screen.findByText('alice');

    expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument();
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

  it('puts the circle first within each network', async () => {
    warpnetService.circleFirst.mockImplementation((users) => Promise.resolve([...(users || [])].reverse()));
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([
        { id: WARPNET_ID, username: 'alice' },
        { id: '01ARZ3NDEKTSV4RRFFQ69G5FAW', username: 'carol' },
      ]),
      mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
    });

    renderView();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    const names = screen.getAllByRole('listitem').map((el) => el.textContent);
    expect(names.indexOf('carol')).toBeLessThan(names.indexOf('alice'));
    expect(warpnetService.circleFirst).toHaveBeenCalledTimes(2);
  });

  describe('when the node misbehaves', () => {
    const showMore = () => screen.getByRole('button', { name: 'Show More' });

    it('shows the empty state when the first request fails', async () => {
      warpnetService.getWhoToFollow.mockRejectedValueOnce(new Error('node down'));

      renderView();

      expect(await screen.findByText('No results')).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument();
    });

    it('keeps its cursors when a next page fails, so it can try again', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockRejectedValueOnce(new Error('timeout'))
        .mockResolvedValueOnce({ warpnet: page([{ id: '01ARZ3NDEKTSV4RRFFQ69G5FAW', username: 'carol' }]) });

      renderView();
      await screen.findByText('alice');

      await fireEvent.click(showMore());
      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2));
      await waitFor(() => expect(showMore()).not.toBeDisabled());

      await fireEvent.click(showMore());
      expect(await screen.findByText('carol')).toBeInTheDocument();
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, { warpnet: 'w-next' });
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(3, { warpnet: 'w-next' });
    });

    it('sends one request however fast "Show More" is clicked', async () => {
      let release;
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));

      renderView();
      await screen.findByText('alice');

      const button = showMore();
      await fireEvent.click(button);
      await fireEvent.click(button);
      await fireEvent.click(button);
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);

      release({});
      await waitFor(() => expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument());
    });

    it('pages on only the networks that still have a cursor', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({
          warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next'),
          mastodon: page([{ id: MASTODON_ID, username: 'bob' }], 'm-next'),
        })
        .mockResolvedValueOnce({
          warpnet: page([], null),
          mastodon: page([{ id: 'dan@mastodon.social', username: 'dan' }], 'm-more'),
        })
        .mockResolvedValueOnce({ mastodon: page([]) });

      renderView();
      await screen.findByText('alice');

      await fireEvent.click(showMore());
      await screen.findByText('dan');
      await fireEvent.click(showMore());

      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(3));
      expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-more' });
    });

    it('shows an account once when a later page repeats it', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }]) });

      renderView();
      await screen.findByText('alice');
      await fireEvent.click(showMore());

      await waitFor(() => expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument());
      expect(screen.getAllByText('alice')).toHaveLength(1);
    });

    it('keeps what it has when a next page comes back empty', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockResolvedValueOnce({});

      renderView();
      await screen.findByText('alice');
      await fireEvent.click(showMore());

      await waitFor(() => expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument());
      expect(screen.getByText('alice')).toBeInTheDocument();
    });
  });
});
