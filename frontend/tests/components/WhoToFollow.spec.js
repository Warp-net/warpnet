import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getWhoToFollow: vi.fn(),
    isFollowing: vi.fn(),
    getImage: vi.fn(),
  },
}));

import WhoToFollow from '@/components/WhoToFollow.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();

// Valid Crockford-base32 ULIDs (warpnet) and fediverse handles (mastodon).
const wid = (n) => '01ARZ3NDEKTSV4RRFFQ69G5F' + String(n).padStart(2, '0');
const mid = (n) => `user${n}@mastodon.social`;
const warpnetUser = (n) => ({ id: wid(n), username: `warp${n}` });
const mastodonUser = (n) => ({ id: mid(n), username: `masto${n}` });
const threadsUser = (n) => ({ id: `user${n}@threads.net`, username: `thr${n}`, network: 'threads' });
// The node's answer as the service hands it over: {network: {users, cursor}}.
const pages = (byNetwork, cursors = {}) =>
  Object.fromEntries(
    Object.entries(byNetwork).map(([network, users]) => [network, { users, cursor: cursors[network] ?? null }])
  );

// The tabs carry their label as the button title; the icon inside carries the
// aria-label, so the title is what addresses the tab itself.
const openTab = (label) => fireEvent.click(screen.getByRole('tab', { name: label }));

const renderComponent = () =>
  render(WhoToFollow, {
    global: {
      mocks: {
        $router: { push: routerPush },
      },
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
  warpnetService.getWhoToFollow.mockResolvedValue({});
  warpnetService.isFollowing.mockResolvedValue(false);
  warpnetService.getImage.mockResolvedValue('');
});

describe('WhoToFollow.vue (sidebar)', () => {
  it('puts each network behind its own tab in one block', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce(
      pages({
        warpnet: [warpnetUser(1), warpnetUser(2)],
        mastodon: [mastodonUser(1)],
        threads: [threadsUser(1)],
      })
    );

    renderComponent();

    // One block, three tabs, Warpnet showing by default.
    expect(await screen.findByText('warp1')).toBeInTheDocument();
    expect(screen.getAllByText('Who to follow')).toHaveLength(1);
    expect(screen.getByRole('tab', { name: 'Warpnet' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Mastodon' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Threads' })).toBeInTheDocument();
    expect(screen.getByText('warp2')).toBeInTheDocument();
    expect(screen.queryByText('masto1')).not.toBeInTheDocument();
    expect(screen.queryByText('thr1')).not.toBeInTheDocument();

    await openTab('Mastodon');
    expect(await screen.findByText('masto1')).toBeInTheDocument();
    expect(screen.queryByText('warp1')).not.toBeInTheDocument();

    await openTab('Threads');
    expect(await screen.findByText('thr1')).toBeInTheDocument();

    // Every network ran out, so one request was enough.
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(1);
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledWith({}, 10);
  });

  it('gives a network the client does not know its own tab', async () => {
    const other = { id: 'bob@bsky.social', username: 'other1', network: 'bluesky' };
    warpnetService.getWhoToFollow.mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1)], bluesky: [other] }));

    renderComponent();

    expect(await screen.findByText('warp1')).toBeInTheDocument();
    expect(screen.queryByText('other1')).not.toBeInTheDocument(); // not under Warpnet
    await openTab('Bluesky');
    expect(await screen.findByText('other1')).toBeInTheDocument();
    expect(screen.queryByText('warp1')).not.toBeInTheDocument();
  });

  it('pages only the short tabs by their own cursor, capping each at 5', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce(
        pages(
          { warpnet: [1, 2, 3, 4, 5, 6].map(warpnetUser), mastodon: [1, 2].map(mastodonUser) },
          { warpnet: 'w-next', mastodon: 'm-next' }
        )
      )
      .mockResolvedValueOnce(pages({ mastodon: [3, 4, 5, 6].map(mastodonUser) }, { mastodon: 'm-more' }));

    renderComponent();

    expect(await screen.findByText('warp5')).toBeInTheDocument();
    expect(screen.queryByText('warp6')).not.toBeInTheDocument(); // capped
    await openTab('Mastodon');
    expect(await screen.findByText('masto5')).toBeInTheDocument();
    expect(screen.queryByText('masto6')).not.toBeInTheDocument();
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
    expect(warpnetService.getWhoToFollow).toHaveBeenLastCalledWith({ mastodon: 'm-next' }, 10);
  });

  it('keeps every tab even when a network has nobody to suggest', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1)] }));

    renderComponent();

    expect(await screen.findByText('warp1')).toBeInTheDocument();
    // A tab that appeared only once it had content would hide the fact that the
    // other networks exist at all, so it stays and says it is empty.
    expect(screen.getByRole('tab', { name: 'Threads' })).toBeInTheDocument();
    await openTab('Threads');
    expect(await screen.findByText('Nobody to suggest here yet.')).toBeInTheDocument();
  });

  it('renders the rows without waiting for hanging avatar blobs', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1), warpnetUser(2)] }));
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));

    renderComponent();

    expect(
      await screen.findByText('warp1', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    expect(screen.getByText('warp2')).toBeInTheDocument();
  });

  it('fills each avatar independently of a failing sibling', async () => {
    warpnetService.getWhoToFollow.mockResolvedValueOnce(
      pages({
        warpnet: [
          { ...warpnetUser(1), avatar_key: 'k1' },
          { ...warpnetUser(2), avatar_key: 'k2' },
        ],
      })
    );
    warpnetService.getImage.mockImplementation(({ key }) =>
      key === 'k1'
        ? Promise.resolve('data:image/png;base64,one')
        : Promise.reject(new Error('blob unavailable'))
    );
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

    renderComponent();

    await waitFor(() => {
      const img = screen.getByAltText('warp1');
      expect(img).toHaveAttribute('src', 'data:image/png;base64,one');
    });
    expect(screen.getByAltText('warp2')).toHaveAttribute('src', '/default_profile.png');
    warnSpy.mockRestore();
  });

  it('stops paging after a bounded number of rounds', async () => {
    let n = 0;
    // Mastodon keeps a cursor but never fills.
    warpnetService.getWhoToFollow.mockImplementation(async () =>
      pages({ warpnet: [warpnetUser(++n)], mastodon: [] }, { mastodon: 'more' })
    );

    renderComponent();

    expect(await screen.findByText('warp1')).toBeInTheDocument();
    await waitFor(() => {
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(5);
    });
  });

  describe('when the node misbehaves', () => {
    it('never asks again for a network that ran out', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1)], mastodon: [mastodonUser(1)] }, { mastodon: 'm-next' }))
        .mockResolvedValueOnce(pages({ mastodon: [mastodonUser(2)] }));

      renderComponent();

      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2));
      expect(warpnetService.getWhoToFollow.mock.calls[1][0]).toEqual({ mastodon: 'm-next' });
    });

    it('never asks again for a tab that is already full', async () => {
      warpnetService.getWhoToFollow.mockResolvedValueOnce(
        pages({ warpnet: [1, 2, 3, 4, 5].map(warpnetUser) }, { warpnet: 'w-next' })
      );

      renderComponent();

      expect(await screen.findByText('warp5')).toBeInTheDocument();
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(1);
    });

    it('never pages the untagged bucket, whatever cursor it carries', async () => {
      warpnetService.getWhoToFollow.mockResolvedValueOnce(
        pages({ '': [{ id: 'old@legacy.example', username: 'legacy1' }] }, { '': 'u-next' })
      );

      renderComponent();

      await openTab('Mastodon');
      expect(await screen.findByText('legacy1')).toBeInTheDocument();
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(1);
    });

    it('shows an account once when the node sends it twice', async () => {
      warpnetService.getWhoToFollow
        .mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1), warpnetUser(1)] }, { warpnet: 'w-next' }))
        .mockResolvedValueOnce(pages({ warpnet: [warpnetUser(1), warpnetUser(2)] }));

      renderComponent();

      expect(await screen.findByText('warp2')).toBeInTheDocument();
      expect(screen.getAllByText('warp1')).toHaveLength(1);
    });

    it('stays bounded when a network key does not match its tab', async () => {
      // the tab key is lower-cased, so the "Mastodon" bucket never fills
      warpnetService.getWhoToFollow.mockImplementation(async () =>
        pages({ Mastodon: [mastodonUser(1)] }, { Mastodon: 'more' })
      );

      renderComponent();

      await openTab('Mastodon');
      expect(await screen.findByText('masto1')).toBeInTheDocument();
      await waitFor(() => expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(5));
      expect(screen.getAllByText('masto1')).toHaveLength(1);
    });

    it('asks the follow state only of the rows it shows', async () => {
      warpnetService.getWhoToFollow.mockResolvedValueOnce(pages({ warpnet: [1, 2, 3, 4, 5, 6, 7].map(warpnetUser) }));

      renderComponent();

      await waitFor(() => expect(warpnetService.isFollowing).toHaveBeenCalledTimes(5));
      expect(warpnetService.isFollowing).not.toHaveBeenCalledWith(wid(6));
    });
  });
});
