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
const WARPNET_ID_2 = '01ARZ3NDEKTSV4RRFFQ69G5FAW';
const MASTODON_ID = 'bob@mastodon.social';
const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
const PAGE = 20;
const page = (users, cursor = null) => ({ users, cursor });
const warpUser = (i) => ({
  id: '01ARZ3NDEKTSV4RRFFQ69G5F' + CROCKFORD[Math.floor(i / 32)] + CROCKFORD[i % 32],
  username: `warp${i}`,
});
const mastoUser = (i) => ({ id: `m${i}@mastodon.social`, username: `masto${i}` });
const range = (from, n, make) => Array.from({ length: n }, (_, k) => make(from + k));
const serve = (first, next = {}) =>
  warpnetService.getWhoToFollow.mockImplementation(async (cursors) => {
    const networks = Object.keys(cursors || {});
    if (networks.length === 0) return first;
    return Object.fromEntries(networks.map((n) => [n, next[cursors[n]] ?? page([])]));
  });
const pagingCalls = () => warpnetService.getWhoToFollow.mock.calls.slice(1).map(([cursors]) => cursors);
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
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledWith({}, PAGE);
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
        warpnet: page(range(0, PAGE, warpUser), 'w-next'),
        mastodon: page(range(0, PAGE, mastoUser), 'm-next'),
      })
      .mockResolvedValueOnce({ mastodon: page(range(PAGE, PAGE, mastoUser), 'm-more') });

    renderView();
    await screen.findByText('warp0');
    await openTab('Mastodon');
    await fireEvent.click(showMore());

    expect(await screen.findByText('masto39')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' }, PAGE);
    expect(shown()).toHaveLength(2 * PAGE);

    await openTab('Warpnet');
    expect(shown()).toHaveLength(PAGE);
    await fireEvent.click(showMore());
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ warpnet: 'w-next' }, PAGE);
  });

  it('keeps the other tabs\' cursors when one tab pages', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({
        warpnet: page(range(0, PAGE, warpUser), 'w-next'),
        mastodon: page(range(0, PAGE, mastoUser), 'm-next'),
      })
      .mockResolvedValueOnce({ warpnet: page(range(PAGE, 3, warpUser)) })
      .mockResolvedValueOnce({ mastodon: page([]) });

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());
    await screen.findByText('warp22');
    await waitFor(noShowMore);

    await openTab('Mastodon');
    await fireEvent.click(showMore());
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' }, PAGE);
  });

  it('hides "Show More" on a tab whose network has run out', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce({
      warpnet: page([{ id: WARPNET_ID, username: 'alice' }]),
      mastodon: page(range(0, PAGE, mastoUser), 'm-next'),
    });

    renderView();
    await screen.findByText('alice');
    noShowMore();

    await openTab('Mastodon');
    expect(showMore()).toBeInTheDocument();
  });

  it('tops up a short first page instead of offering "Show More" for the rest', async () => {
    const thr = (id, username) => ({ id, username, network: 'threads' });
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ threads: page([thr('engineer_of_your_ass@threads.net', 'engineer')], 't-1') })
      .mockResolvedValueOnce({ threads: page([thr('zuck@threads.net', 'zuck')]) });

    renderView();
    await openTab('Threads');

    expect(await screen.findByText('zuck')).toBeInTheDocument();
    expect(shown()).toEqual(['engineer', 'zuck']);
    noShowMore();
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(1, {}, PAGE);
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, { threads: 't-1' }, PAGE);
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
  });

  it('fills a tab whose first page came back empty but with a cursor', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ warpnet: page([], 'w-1'), mastodon: page(range(0, PAGE, mastoUser), 'm-1') })
      .mockResolvedValueOnce({ warpnet: page([], 'w-2') })
      .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-3') });

    renderView();

    expect(await screen.findByText('warp0')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(1, {}, PAGE);
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, { warpnet: 'w-1' }, PAGE);
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(3, { warpnet: 'w-2' }, PAGE);
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(3);
    expect(showMore()).toBeInTheDocument();
  });

  it('fills every short network on its own, one network per request', async () => {
    serve(
      {
        warpnet: page([], 'w-1'),
        mastodon: page([], 'm-1'),
        threads: page(range(0, PAGE, (i) => ({ id: `t${i}@threads.net`, username: `thr${i}`, network: 'threads' })), 't-1'),
        nostr: page([]),
      },
      {
        'w-1': page([{ id: WARPNET_ID, username: 'alice' }]),
        'm-1': page([], 'm-2'),
        'm-2': page([{ id: MASTODON_ID, username: 'bob' }]),
      },
    );

    renderView();

    expect(await screen.findByText('alice')).toBeInTheDocument();
    await openTab('Mastodon');
    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(pagingCalls()).toEqual(expect.arrayContaining([{ warpnet: 'w-1' }, { mastodon: 'm-1' }, { mastodon: 'm-2' }]));
    expect(pagingCalls()).toHaveLength(3);
    for (const cursors of pagingCalls()) {
      expect(Object.keys(cursors)).toHaveLength(1);
    }
  });

  it('does not let a slow network hold up the others', async () => {
    let releaseWarpnet;
    warpnetService.getWhoToFollow.mockImplementation(async (cursors) => {
      if (!cursors || Object.keys(cursors).length === 0) {
        return { warpnet: page([], 'w-1'), mastodon: page([{ id: MASTODON_ID, username: 'bob' }], 'm-1') };
      }
      if (cursors.warpnet) return new Promise((resolve) => { releaseWarpnet = resolve; });
      return { mastodon: page(range(0, PAGE, mastoUser)) };
    });

    renderView();
    await openTab('Mastodon');

    expect(await screen.findByText('masto19')).toBeInTheDocument();
    await openTab('Warpnet');
    expect(screen.queryByText('no-users')).not.toBeInTheDocument();

    releaseWarpnet({ warpnet: page([{ id: WARPNET_ID, username: 'alice' }]) });
    expect(await screen.findByText('alice')).toBeInTheDocument();
  });

  it('keeps "Show More" of one tab usable while another tab loads', async () => {
    let releaseWarpnet;
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({
        warpnet: page(range(0, PAGE, warpUser), 'w-1'),
        mastodon: page(range(0, PAGE, mastoUser), 'm-1'),
      })
      .mockImplementationOnce(() => new Promise((resolve) => { releaseWarpnet = resolve; }))
      .mockResolvedValueOnce({ mastodon: page(range(PAGE, PAGE, mastoUser)) });

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());
    expect(showMore()).toBeDisabled();

    await openTab('Mastodon');
    expect(showMore()).not.toBeDisabled();
    await fireEvent.click(showMore());
    expect(await screen.findByText('masto39')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-1' }, PAGE);

    releaseWarpnet({ warpnet: page(range(PAGE, PAGE, warpUser)) });
    await openTab('Warpnet');
    expect(await screen.findByText('warp39')).toBeInTheDocument();
  });

  it('stops filling after a bounded number of rounds', async () => {
    warpnetService.getWhoToFollow.mockImplementation(async () => ({ warpnet: page([], 'w-more') }));

    renderView();

    await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(6));
    expect(await screen.findByText('no-users')).toBeInTheDocument();
    expect(showMore()).toBeInTheDocument();
  });

  it('lets "Show More" top up short pages to a full one', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-1') })
      .mockResolvedValueOnce({ warpnet: page(range(PAGE, 5, warpUser), 'w-2') })
      .mockResolvedValueOnce({ warpnet: page(range(PAGE + 5, 15, warpUser), 'w-3') });

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());

    expect(await screen.findByText('warp39')).toBeInTheDocument();
    await waitFor(() => expect(showMore()).not.toBeDisabled());
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(3);
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ warpnet: 'w-2' }, PAGE);
  });

  it('lets "Show More" skip pages that come back empty', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-1') })
      .mockResolvedValueOnce({ warpnet: page([], 'w-2') })
      .mockResolvedValueOnce({ warpnet: page([], 'w-3') })
      .mockResolvedValueOnce({ warpnet: page(range(PAGE, PAGE, warpUser), 'w-4') });

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());

    expect(await screen.findByText('warp39')).toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(4);
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ warpnet: 'w-3' }, PAGE);
  });

  it('stops a "Show More" that finds nobody after a bounded number of rounds', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-1') })
      .mockImplementation(async () => ({ warpnet: page([], 'w-more') }));

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());

    await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(6));
    await waitFor(() => expect(showMore()).not.toBeDisabled());
  });

  it('stops a "Show More" as soon as the network ends', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-1') })
      .mockResolvedValueOnce({ warpnet: page([warpUser(PAGE)]) });

    renderView();
    await screen.findByText('warp0');
    await fireEvent.click(showMore());

    expect(await screen.findByText('warp20')).toBeInTheDocument();
    await waitFor(noShowMore);
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
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
        .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-next') })
        .mockRejectedValueOnce(new Error('timeout'))
        .mockResolvedValueOnce({ warpnet: page([{ id: WARPNET_ID_2, username: 'carol' }]) });

      renderView();
      await screen.findByText('warp0');

      await fireEvent.click(showMore());
      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2));
      await waitFor(() => expect(showMore()).not.toBeDisabled());

      await fireEvent.click(showMore());
      expect(await screen.findByText('carol')).toBeInTheDocument();
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, { warpnet: 'w-next' }, PAGE);
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(3, { warpnet: 'w-next' }, PAGE);
    });

    it('sends one request however fast "Show More" is clicked', async () => {
      let release;
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-next') })
        .mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));

      renderView();
      await screen.findByText('warp0');

      const button = showMore();
      await fireEvent.click(button);
      await fireEvent.click(button);
      await fireEvent.click(button);
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);

      release({ warpnet: page(range(PAGE, PAGE, warpUser)) });
      await waitFor(noShowMore);
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
    });

    it('does not let a slow page of one tab land in another', async () => {
      let release;
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({
          warpnet: page(range(0, PAGE, warpUser), 'w-next'),
          mastodon: page([{ id: MASTODON_ID, username: 'bob' }]),
        })
        .mockImplementationOnce(() => new Promise((resolve) => { release = resolve; }));

      renderView();
      await screen.findByText('warp0');
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
        .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-next') })
        .mockResolvedValueOnce({ warpnet: page([warpUser(0)]) });

      renderView();
      await screen.findByText('warp0');
      await fireEvent.click(showMore());

      await waitFor(noShowMore);
      expect(screen.getAllByText('warp0')).toHaveLength(1);
      expect(shown()).toHaveLength(PAGE);
    });

    it('keeps what it has when a next page comes back empty', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce({ warpnet: page(range(0, PAGE, warpUser), 'w-next') })
        .mockResolvedValue({});

      renderView();
      await screen.findByText('warp0');
      await fireEvent.click(showMore());

      await waitFor(() => expect(showMore()).not.toBeDisabled());
      expect(shown()).toHaveLength(PAGE);
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(6);
    });
  });
});
