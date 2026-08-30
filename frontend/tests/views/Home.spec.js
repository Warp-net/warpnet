import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';
import { reactive } from 'vue';

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
      // The local page now paints before created() finishes; let it settle
      // (register the poll timer) before advancing the fake clock.
      await new Promise((r) => setTimeout(r, 0));

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

  it('keeps the poll quiet when mastodon rows fill the whole first page', async () => {
    // Followed Mastodon accounts out-post the local network: page 1 holds no
    // warpnet rows at all. The 10s poll must still not resurface the stale
    // warpnet backlog on top — the reference is the merger's warpnet source,
    // not the visible feed.
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    try {
      warpnetService.getMyTimeline.mockResolvedValue([
        wTweet('w1', '2026-01-01T10:00:00Z'),
        wTweet('w2', '2026-01-01T09:00:00Z'),
      ]);
      warpnetService.listFollowingIds.mockResolvedValue(['bob@mastodon.social']);
      warpnetService.getUserTweetsPage.mockResolvedValue({
        tweets: Array.from({ length: 20 }, (_, n) =>
          mTweet(`m${n}`, `2026-01-02T10:${String(59 - n).padStart(2, '0')}:00Z`)),
        cursor: 'end',
      });

      renderHome();
      await waitFor(() => expect(screen.getByText('mastodon m0')).toBeInTheDocument());
      // The local page now paints before created() finishes; let the merged
      // replacement and the poll-timer registration settle.
      await new Promise((r) => setTimeout(r, 0));
      expect(screen.queryByText('warpnet w1')).not.toBeInTheDocument();

      vi.advanceTimersByTime(10000);
      await waitFor(() => expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(2));
      await new Promise((r) => setTimeout(r, 0));

      // stale warpnet rows stay off the top; the scroll path will emit them
      const rows = screen.getAllByRole('article').map((el) => el.textContent.trim());
      expect(rows[0]).toBe('mastodon m0');
      expect(screen.queryByText('warpnet w1')).not.toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it('drops boosts that echo the owner’s own federated tweet', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-03T10:00:00Z')]);
    warpnetService.listFollowingIds.mockResolvedValue(['bob@mastodon.social']);
    warpnetService.getUserTweetsPage.mockResolvedValue({
      tweets: [
        mTweet('m1', '2026-01-02T10:00:00Z'),
        {
          id: 'https://gw.ts.net/users/owner1/statuses/w9',
          user_id: 'owner1@gw.ts.net',
          text: 'my own tweet echoed back',
          created_at: '2026-01-04T10:00:00Z',
          network: 'mastodon',
          retweeted_by: 'bob@mastodon.social',
        },
      ],
      cursor: 'end',
    });

    renderHome();

    await waitFor(() => expect(screen.getByText('mastodon m1')).toBeInTheDocument());
    expect(screen.queryByText('my own tweet echoed back')).not.toBeInTheDocument();
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

describe('Home first paint under hanging elements', () => {
  it('renders the local timeline without waiting for the owner profile assets', async () => {
    warpnetService.getProfile.mockImplementation(() => new Promise(() => {}));
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);

    renderHome();

    expect(await screen.findByText('warpnet w1')).toBeInTheDocument();
  });

  it('paints the local page while a bridged source hangs', async () => {
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);
    warpnetService.listFollowingIds.mockResolvedValue(['bob@mastodon.social']);
    warpnetService.getUserTweetsPage.mockImplementation(() => new Promise(() => {}));

    renderHome();

    expect(
      await screen.findByText('warpnet w1', undefined, { timeout: 3000 }),
    ).toBeInTheDocument();
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

// The 10s poll only prepends tweets it has not seen, so a row already on
// screen keeps the counters it was rendered with — a like or a vote landing
// from another client never shows up. Clicking Home stamps ?refresh, and that
// has to reload the first page outright.
describe('Home manual refresh', () => {
  it('reloads the first page when ?refresh changes', async () => {
    const query = reactive({});
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);

    renderHome(query);
    await waitFor(() => expect(screen.getByText('warpnet w1')).toBeInTheDocument());
    expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(1);

    query.refresh = '1700000000000';

    await waitFor(() => expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(2));
  });

  it('replaces rows already on screen rather than only prepending', async () => {
    const query = reactive({});
    const stale = { ...wTweet('w1', '2026-01-02T10:00:00Z'), text: 'warpnet stale' };
    const fresh = { ...wTweet('w1', '2026-01-02T10:00:00Z'), text: 'warpnet fresh' };
    warpnetService.getMyTimeline.mockResolvedValue([stale]);

    renderHome(query);
    await waitFor(() => expect(screen.getByText('warpnet stale')).toBeInTheDocument());

    warpnetService.getMyTimeline.mockResolvedValue([fresh]);
    query.refresh = '1700000000000';

    await waitFor(() => expect(screen.getByText('warpnet fresh')).toBeInTheDocument());
    expect(screen.queryByText('warpnet stale')).not.toBeInTheDocument();
  });

  it('reloads again on a second, different stamp', async () => {
    const query = reactive({});
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);

    renderHome(query);
    await waitFor(() => expect(screen.getByText('warpnet w1')).toBeInTheDocument());

    // Wait for each reload to land before asking for the next one: a stamp
    // arriving mid-reload is dropped on purpose.
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w2', '2026-01-02T11:00:00Z')]);
    query.refresh = '1700000000000';
    await waitFor(() => expect(screen.getByText('warpnet w2')).toBeInTheDocument());

    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w3', '2026-01-02T12:00:00Z')]);
    query.refresh = '1700000000001';
    await waitFor(() => expect(screen.getByText('warpnet w3')).toBeInTheDocument());

    expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(3);
  });

  it('ignores a cleared stamp', async () => {
    const query = reactive({ refresh: '1700000000000' });
    warpnetService.getMyTimeline.mockResolvedValue([wTweet('w1', '2026-01-02T10:00:00Z')]);

    renderHome(query);
    await waitFor(() => expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(1));

    query.refresh = '';
    await new Promise((r) => setTimeout(r, 0));

    expect(warpnetService.getMyTimeline).toHaveBeenCalledTimes(1);
  });
});
