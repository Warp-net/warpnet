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

// The tabs carry their label as the button title; the icon inside carries the
// aria-label, so the title is what addresses the tab itself.
const openTab = (label) => fireEvent.click(screen.getByTitle(label));

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
  warpnetService.getWhoToFollow.mockResolvedValue([]);
  warpnetService.isFollowing.mockResolvedValue(false);
  warpnetService.getImage.mockResolvedValue('');
});

describe('WhoToFollow.vue (sidebar)', () => {
  it('puts each network behind its own tab in one block', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([warpnetUser(1), mastodonUser(1), threadsUser(1), warpnetUser(2)])
      .mockResolvedValueOnce([]);

    renderComponent();

    // One block, three tabs, Warpnet showing by default.
    expect(await screen.findByText('warp1')).toBeInTheDocument();
    expect(screen.getAllByText('Who to follow')).toHaveLength(1);
    expect(screen.getByTitle('Warpnet')).toBeInTheDocument();
    expect(screen.getByTitle('Mastodon')).toBeInTheDocument();
    expect(screen.getByTitle('Threads')).toBeInTheDocument();
    expect(screen.getByText('warp2')).toBeInTheDocument();
    expect(screen.queryByText('masto1')).not.toBeInTheDocument();
    expect(screen.queryByText('thr1')).not.toBeInTheDocument();

    await openTab('Mastodon');
    expect(await screen.findByText('masto1')).toBeInTheDocument();
    expect(screen.queryByText('warp1')).not.toBeInTheDocument();

    await openTab('Threads');
    expect(await screen.findByText('thr1')).toBeInTheDocument();

    await waitFor(() => {
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, false, 10);
    });
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(1, true, 10);
  });

  it('keeps paging until each tab is filled, capping each at 5', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([1, 2, 3, 4, 5, 6].map(warpnetUser))
      .mockResolvedValueOnce([1, 2, 3, 4, 5].map(mastodonUser))
      .mockResolvedValueOnce([]);

    renderComponent();

    expect(await screen.findByText('warp5')).toBeInTheDocument();
    expect(screen.queryByText('warp6')).not.toBeInTheDocument(); // capped
    await openTab('Mastodon');
    expect(await screen.findByText('masto5')).toBeInTheDocument();
  });

  it('keeps every tab even when a network has nobody to suggest', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([warpnetUser(1)])
      .mockResolvedValueOnce([]);

    renderComponent();

    expect(await screen.findByText('warp1')).toBeInTheDocument();
    // A tab that appeared only once it had content would hide the fact that the
    // other networks exist at all, so it stays and says it is empty.
    expect(screen.getByTitle('Threads')).toBeInTheDocument();
    await openTab('Threads');
    expect(await screen.findByText('Nobody to suggest here yet.')).toBeInTheDocument();
  });

  it('renders the rows without waiting for hanging avatar blobs', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([warpnetUser(1), warpnetUser(2)])
      .mockResolvedValueOnce([]);
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));

    renderComponent();

    expect(
      await screen.findByText('warp1', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    expect(screen.getByText('warp2')).toBeInTheDocument();
  });

  it('fills each avatar independently of a failing sibling', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([
        { ...warpnetUser(1), avatar_key: 'k1' },
        { ...warpnetUser(2), avatar_key: 'k2' },
      ])
      .mockResolvedValueOnce([]);
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
    // Endless warpnet-only feed: mastodon never fills.
    warpnetService.getWhoToFollow.mockImplementation(async () =>
      [1, 2].map(() => warpnetUser(++n))
    );

    renderComponent();

    expect(await screen.findByText('warp1')).toBeInTheDocument();
    await waitFor(() => {
      expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(5);
    });
  });
});
