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
const WARPNET_ID_2 = '01ARZ3NDEKTSV4RRFFQ69G5FAW';
const page = (users, cursor = null) => ({ users, cursor });
const openTab = (label) => fireEvent.click(screen.getByRole('tab', { name: label }));
const shown = () => screen.getAllByRole('listitem').map((el) => el.textContent);
const showMore = () => screen.getByRole('button', { name: 'Show More' });
const noShowMore = () => expect(screen.queryByRole('button', { name: 'Show More' })).not.toBeInTheDocument();

describe('WhoToFollow.vue', () => {
  it('puts each network behind its own tab, Warpnet first', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
    });

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.queryByText('bob')).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Warpnet' })).toHaveAttribute('aria-selected', 'true');

    await openTab('Mastodon');
    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(screen.queryByText('alice')).not.toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(1);
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledWith();
  });

  it('offers every known network as a tab, empty or not', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }]) });

    renderView();
    await screen.findByText('alice');

    for (const label of ['Warpnet', 'Mastodon', 'Threads', 'Nostr']) {
      expect(screen.getByRole('tab', { name: label })).toBeInTheDocument();
    }
    await openTab('Threads');
    expect(await screen.findByText('no-users')).toBeInTheDocument();
    expect(screen.queryByText('No results')).not.toBeInTheDocument();
  });

  it('gives a network the client does not know its own tab', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      bluesky: page([{ id: 'carol@bsky.social', username: 'carol', network: 'bluesky' }]),
    });

    renderView();
    await screen.findByText('alice');

    await openTab('Bluesky');
    expect(await screen.findByText('carol')).toBeInTheDocument();
  });

  it('shows the empty state when there are no recommendations', async () => {
    renderView();

    expect(await screen.findByText('no-users')).toBeInTheDocument();
    noShowMore();
  });

  it('pages only the open tab, by its own cursor', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({
        warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next'),
        mastodon: page([{ id: MASTODON_ID, username: 'bob' }], 'm-next'),
      })
      .mockResolvedValueOnce({ mastodon: page([{ id: 'dan@mastodon.social', username: 'dan' }], 'm-more') });

    renderView();
    await screen.findByText('alice');
    await openTab('Mastodon');
    await fireEvent.click(showMore());

    expect(await screen.findByText('dan')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' });
    expect(shown()).toEqual(['bob', 'dan']);

    await openTab('Warpnet');
    expect(shown()).toEqual(['alice']);
    await fireEvent.click(showMore());
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ warpnet: 'w-next' });
  });

  it('keeps the other tabs\' cursors when one tab pages', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({
        warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next'),
        mastodon: page([{ id: MASTODON_ID, username: 'bob' }], 'm-next'),
      })
      .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID_2, username: 'carol' }]) })
      .mockResolvedValueOnce({ mastodon: page([]) });

    renderView();
    await screen.findByText('alice');
    await fireEvent.click(showMore());
    await screen.findByText('carol');
    await waitFor(noShowMore);

    await openTab('Mastodon');
    await fireEvent.click(showMore());
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' });
  });

  it('hides "Show More" on a tab whose network has run out', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      mastodon: page([{ id: MASTODON_ID, username: 'bob' }], 'm-next'),
    });

    renderView();
    await screen.findByText('alice');
    noShowMore();

    await openTab('Mastodon');
    expect(showMore()).toBeInTheDocument();
  });

  it('offers "Show More" on an empty tab that still has a cursor', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ threads: page([], 't-next') })
      .mockResolvedValueOnce({ threads: page([{ id: 'me@threads.net', username: 'thr', network: 'threads' }]) });

    renderView();
    await screen.findByText('no-users');
    await openTab('Threads');
    await fireEvent.click(showMore());

    expect(await screen.findByText('thr')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ threads: 't-next' });
  });

  it('navigates home when the back button is clicked', async () => {
    renderView();
    await screen.findByText('no-users');

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
        { id: WARPNET_ID_2, username: 'carol' },
      ]),
      mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
    });

    renderView();

    expect(await screen.findByText('carol')).toBeInTheDocument();
    expect(shown()).toEqual(['carol', 'alice']);
    expect(warpnetService.circleFirst).toHaveBeenCalledTimes(2);
  });

  describe('when the node misbehaves', () => {
    it('shows the empty state when the first request fails', async () => {
      warpnetService.getWhoToFollow.mockRejectedValueOnce(new Error('node down'));

      renderView();

      expect(await screen.findByText('no-users')).toBeInTheDocument();
      noShowMore();
    });

    it('keeps the cursor when a next page fails, so it can try again', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockRejectedValueOnce(new Error('timeout'))
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID_2, username: 'carol' }]) });

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
      await waitFor(() => expect(showMore()).not.toBeDisabled());
    });

    it('does not let a slow page of one tab land in another', async () => {
      let release;
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({
          warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next'),
          mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
        })
        .mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));

      renderView();
      await screen.findByText('alice');
      await fireEvent.click(showMore());
      await openTab('Mastodon');

      release({ warpnet: page([{ id: WARPNET_ID_2, username: 'carol' }]) });
      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2));
      expect(shown()).toEqual(['bob']);
      await openTab('Warpnet');
      expect(await screen.findByText('carol')).toBeInTheDocument();
    });

    it('shows an account once when a later page repeats it', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }]) });

      renderView();
      await screen.findByText('alice');
      await fireEvent.click(showMore());

      await waitFor(noShowMore);
      expect(screen.getAllByText('alice')).toHaveLength(1);
    });

    it('keeps what it has when a next page comes back empty', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }], 'w-next') })
        .mockResolvedValueOnce({});

      renderView();
      await screen.findByText('alice');
      await fireEvent.click(showMore());

      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2));
      expect(screen.getByText('alice')).toBeInTheDocument();
    });
  });
});
