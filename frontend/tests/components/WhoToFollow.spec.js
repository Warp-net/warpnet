import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

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
  it('splits a mixed batch into Warpnet and Mastodon blocks', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([warpnetUser(1), mastodonUser(1), warpnetUser(2)])
      .mockResolvedValueOnce([]);

    renderComponent();

    expect(await screen.findByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.getByLabelText('Mastodon')).toBeInTheDocument();
    expect(screen.getAllByText('Who to follow')).toHaveLength(2);
    expect(screen.getByText('warp1')).toBeInTheDocument();
    expect(screen.getByText('warp2')).toBeInTheDocument();
    expect(screen.getByText('masto1')).toBeInTheDocument();
    await waitFor(() => {
      expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(2, false, 10);
    });
    expect(warpnetService.getWhoToFollow).toHaveBeenNthCalledWith(1, true, 10);
  });

  it('keeps paging until both blocks are filled, capping each at 5', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([1, 2, 3, 4, 5, 6].map(warpnetUser))
      .mockResolvedValueOnce([1, 2, 3, 4, 5].map(mastodonUser));

    renderComponent();

    expect(await screen.findByText('warp5')).toBeInTheDocument();
    expect(await screen.findByText('masto5')).toBeInTheDocument();
    expect(screen.queryByText('warp6')).not.toBeInTheDocument(); // capped
    expect(warpnetService.getWhoToFollow).toHaveBeenCalledTimes(2);
  });

  it('hides the Mastodon block when the feed has no mastodon users', async () => {
    warpnetService.getWhoToFollow
      .mockResolvedValueOnce([warpnetUser(1)])
      .mockResolvedValueOnce([]);

    renderComponent();

    expect(await screen.findByAltText('Warpnet')).toBeInTheDocument();
    expect(screen.queryByLabelText('Mastodon')).not.toBeInTheDocument();
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
