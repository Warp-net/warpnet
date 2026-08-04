import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

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
  },
}));

import Home from '@/views/Home.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = { mounted() {}, updated() {}, unmounted() {} };

const tweetsStub = {
  props: ['tweets'],
  template: '<div><article v-for="t in tweets" :key="t.id">{{ t.text }}</article></div>',
};

const renderHome = (query = {}) =>
  render(Home, {
    global: {
      mocks: {
        $router: { push: vi.fn() },
        $route: { query },
      },
      directives: { scroll: scrollDirective },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: true,
        InfoOverlay: true,
        AltTextModal: true,
        ImportTweetsModal: true,
        EmojiPicker: true,
        Tweets: tweetsStub,
      },
    },
  });

const wTweet = (id, iso) => ({
  id,
  user_id: 'owner1',
  text: `warpnet ${id}`,
  created_at: iso,
  network: 'warpnet',
});
const mTweet = (id, iso) => ({
  id: `https://mastodon.social/statuses/${id}`,
  user_id: 'bob@mastodon.social',
  text: `mastodon ${id}`,
  created_at: iso,
  network: 'mastodon',
});

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  Element.prototype.scrollIntoView = vi.fn();
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
  warpnetService.getNodeInfo.mockResolvedValue({});
});

describe('Home unified timeline', () => {
  it('keeps the plain path when no fediverse account is followed', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);
    warpnetService.listFollowingIds.mockResolvedValue(['01ARZ3NDEKTSV4RRFFQ69G5FAV']);

    renderHome();

    await waitFor(() => expect(screen.getByText('warpnet w1')).toBeInTheDocument());
    expect(warpnetService.getUserTweetsPage).not.toHaveBeenCalled();
  });

  it('interleaves followed Mastodon posts with the local timeline by date', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([
      wTweet('w1', '2026-01-03T10:00:00Z'),
      wTweet('w2', '2026-01-01T10:00:00Z'),
    ]);
    warpnetService.listFollowingIds.mockResolvedValue([
      '01ARZ3NDEKTSV4RRFFQ69G5FAV',
      'bob@mastodon.social',
    ]);
    warpnetService.getUserTweetsPage.mockResolvedValue({
      tweets: [mTweet('m1', '2026-01-02T10:00:00Z')],
      cursor: 'end',
    });

    renderHome();

    await waitFor(() => expect(screen.getByText('mastodon m1')).toBeInTheDocument());
    const rows = screen.getAllByRole('article').map((el) => el.textContent.trim());
    expect(rows).toEqual(['warpnet w1', 'mastodon m1', 'warpnet w2']);
    expect(warpnetService.getUserTweetsPage).toHaveBeenCalledWith(
      expect.objectContaining({ userId: 'bob@mastodon.social' }),
    );
  });

  it('still renders the local timeline when the bridged source fails', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);
    warpnetService.listFollowingIds.mockResolvedValue(['bob@mastodon.social']);
    warpnetService.getUserTweetsPage.mockRejectedValue(new Error('gateway down'));

    renderHome();

    await waitFor(() => expect(screen.getByText('warpnet w1')).toBeInTheDocument());
  });

  it('does not let the poll resurface stale warpnet tweets displaced by mastodon rows', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    try {
      warpnetService.getMyTimeline
        // initial page: the merged view will hold w1 + m1
        .mockResolvedValueOnce([wTweet('w1', '2026-01-05T10:00:00Z')])
        // first poll: 'stale' is unseen but OLDER than w1 — a tweet the
        // mastodon rows displaced off the merged first page
        .mockResolvedValueOnce([
          wTweet('w1', '2026-01-05T10:00:00Z'),
          wTweet('stale', '2026-01-03T10:00:00Z'),
        ])
        // later polls: a genuinely new tweet
        .mockResolvedValue([
          wTweet('fresh', '2026-01-06T10:00:00Z'),
          wTweet('w1', '2026-01-05T10:00:00Z'),
        ]);
      warpnetService.listFollowingIds.mockResolvedValue(['bob@mastodon.social']);
      warpnetService.getUserTweetsPage.mockResolvedValue({
        tweets: [mTweet('m1', '2026-01-04T10:00:00Z')],
        cursor: 'end',
      });

      renderHome();
      await waitFor(() => expect(screen.getByText('mastodon m1')).toBeInTheDocument());

      vi.advanceTimersByTime(10000);
      await waitFor(() => expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(2));
      await new Promise((r) => setTimeout(r, 0));
      expect(screen.queryByText('warpnet stale')).not.toBeInTheDocument();

      vi.advanceTimersByTime(10000);
      await waitFor(() => expect(screen.getByText('warpnet fresh')).toBeInTheDocument());
      expect(screen.queryByText('warpnet stale')).not.toBeInTheDocument();
      const rows = screen.getAllByRole('article').map((el) => el.textContent.trim());
      expect(rows[0]).toBe('warpnet fresh');
    } finally {
      vi.useRealTimers();
    }
  });

  it('leaves blocked and muted bridged handles out of the fan-out', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);
    warpnetService.listFollowingIds.mockResolvedValue([
      'bob@mastodon.social',
      'spammer@bad.social',
    ]);
    warpnetService.isUserBlocked.mockImplementation(async (id) => id === 'spammer@bad.social');
    warpnetService.getUserTweetsPage.mockResolvedValue({ tweets: [], cursor: 'end' });

    renderHome();

    await waitFor(() => expect(screen.getByText('warpnet w1')).toBeInTheDocument());
    await waitFor(() => expect(warpnetService.getUserTweetsPage).toHaveBeenCalled());
    const handles = warpnetService.getUserTweetsPage.mock.calls.map((c) => c[0].userId);
    expect(handles).toContain('bob@mastodon.social');
    expect(handles).not.toContain('spammer@bad.social');
  });
});

describe('Home composer watchers', () => {
  it('clamps the composer to 280 runes', async () => {
    renderHome();
    const box = await screen.findByLabelText('Compose a tweet');

    await fireEvent.update(box, 'x'.repeat(300));

    await waitFor(() => expect(box.value).toHaveLength(280));
  });

  it('focuses the composer when arriving with ?compose', async () => {
    renderHome({ compose: '1' });
    const box = await screen.findByLabelText('Compose a tweet');

    await waitFor(() => expect(document.activeElement).toBe(box));
  });
});
