import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getMyTimeline: vi.fn(),
    getCursor: vi.fn(),
    setCursor: vi.fn(),
    listFollowingIds: vi.fn(),
    getUserTweetsPage: vi.fn(),
    applyHomeFilters: vi.fn(),
    isUserBlocked: vi.fn(),
    isUserMuted: vi.fn(),
    consumePendingDeepLink: vi.fn(),
    getNodeInfo: vi.fn(),
    getOwnRating: vi.fn(),
  },
}));

import Home from '@/views/Home.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = { mounted() {}, updated() {}, unmounted() {} };

const renderHome = () =>
  render(Home, {
    global: {
      mocks: {
        $router: { push: vi.fn() },
        $route: { query: {} },
      },
      directives: { scroll: scrollDirective },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: true,
        AltTextModal: true,
        ImportTweetsModal: true,
        EmojiPicker: true,
        Tweets: true,
      },
    },
  });

const text = (el) => el.textContent.replace(/\s+/g, ' ').trim();

// Opens the node info panel the way a user does and returns its rating row.
const openInfo = async () => {
  renderHome();
  await fireEvent.click(await screen.findByRole('button', { name: 'Show node info' }));
  return (await screen.findByText('rating:')).parentElement;
};

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  warnSpy.mockRestore();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'owner1', username: 'Owner' });
  warpnetService.getProfile.mockResolvedValue({ id: 'owner1', username: 'Owner' });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.getMyTimeline.mockResolvedValue([]);
  warpnetService.getCursor.mockReturnValue('end');
  warpnetService.listFollowingIds.mockResolvedValue([]);
  warpnetService.getUserTweetsPage.mockResolvedValue({ tweets: [], cursor: 'end' });
  warpnetService.applyHomeFilters.mockImplementation(async (tweets) => tweets);
  warpnetService.isUserBlocked.mockResolvedValue(false);
  warpnetService.isUserMuted.mockResolvedValue(false);
  warpnetService.consumePendingDeepLink.mockResolvedValue(null);
  warpnetService.getNodeInfo.mockResolvedValue({
    version: '0.8.51',
    network_state: 'Connected',
    peers_online: 7,
  });
  warpnetService.getOwnRating.mockResolvedValue(null);
});

describe('Home node info rating line', () => {
  it.each([
    [{ overall: 742, tier: 'watched', observers: 3 }, '742/1000 watched (3 observers)'],
    [{ overall: 1000, tier: 'trusted', observers: 1 }, '1000/1000 trusted (1 observer)'],
    [{ overall: 1000, tier: 'trusted', observers: 0 }, '1000/1000 trusted (0 observers)'],
    [{ overall: 910, tier: 'trusted', observers: 2 }, '910/1000 trusted (2 observers)'],
    [{ overall: 0, tier: 'floor', observers: 4 }, '0/1000 floor (4 observers)'],
  ])('reads %o as "%s"', async (resp, line) => {
    warpnetService.getOwnRating.mockResolvedValue({
      node_id: '12D3KooWOwnerNode',
      dimensions: null,
      updated_at: '2026-09-30T00:00:00Z',
      ...resp,
    });

    expect(text(await openInfo())).toBe(`rating: ${line}`);
  });

  it.each([
    ['no answer', null],
    ['an empty answer', {}],
    ['an error body', { code: 500, message: 'rating is not available on this node' }],
    ['an answer without overall', { tier: 'trusted', observers: 2 }],
  ])('reads "unavailable" on %s', async (_, resp) => {
    warpnetService.getOwnRating.mockResolvedValue(resp);

    expect(text(await openInfo())).toBe('rating: unavailable');
  });

  it('reads "unavailable" and logs why when the request fails, keeping the rest of the panel', async () => {
    const err = new Error('stream reset');
    warpnetService.getOwnRating.mockRejectedValue(err);

    expect(text(await openInfo())).toBe('rating: unavailable');
    expect(text(screen.getByText('version:').parentElement)).toBe('version: 0.8.51');
    expect(text(screen.getByText('peers_online:').parentElement)).toBe('peers_online: 7');
    expect(errSpy).toHaveBeenCalledWith('get own rating:', err);
  });
});
