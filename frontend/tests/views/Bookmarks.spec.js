import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getBookmarks: vi.fn(),
    getTweet: vi.fn(),
    getOwnerProfile: vi.fn(),
  },
}));

import Bookmarks from '@/views/Bookmarks.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const renderBookmarks = () =>
  render(Bookmarks, {
    global: {
      mocks: {
        $router: { back: vi.fn(), push: vi.fn() },
      },
      directives: { scroll: scrollDirective },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: {
          props: ['loading'],
          template: '<div v-if="loading" data-testid="loader" />',
        },
        TweetBlock: {
          props: ['tweet'],
          template: '<div>{{ tweet.text }}</div>',
        },
      },
    },
  });

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
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'me' });
  warpnetService.getBookmarks.mockResolvedValue({ items: [], cursor: 'end' });
  warpnetService.getTweet.mockResolvedValue(null);
});

describe('Bookmarks.vue', () => {
  it('shows the empty state when there are no bookmarks', async () => {
    renderBookmarks();
    expect(await screen.findByText(/Save tweets for later/i)).toBeInTheDocument();
  });

  it('renders a bookmarked tweet once hydrated', async () => {
    warpnetService.getBookmarks.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    warpnetService.getTweet.mockResolvedValue({ id: 't1', text: 'saved tweet' });

    renderBookmarks();

    expect(await screen.findByText('saved tweet')).toBeInTheDocument();
    expect(warpnetService.getTweet).toHaveBeenCalledWith({ userId: 'u1', tweetId: 't1' });
  });

  it('clears the loader even when tweet hydration hangs', async () => {
    warpnetService.getBookmarks.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    warpnetService.getTweet.mockImplementation(() => new Promise(() => {}));

    renderBookmarks();

    await waitFor(
      () => expect(screen.queryByTestId('loader')).not.toBeInTheDocument(),
      { timeout: 3000 },
    );
  });

  it('fills the tweet in place when hydration resolves late', async () => {
    warpnetService.getBookmarks.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    let resolveTweet;
    warpnetService.getTweet.mockImplementation(
      () => new Promise((resolve) => { resolveTweet = resolve; }),
    );

    renderBookmarks();

    await waitFor(
      () => expect(screen.queryByTestId('loader')).not.toBeInTheDocument(),
      { timeout: 3000 },
    );
    expect(screen.queryByText('late tweet')).not.toBeInTheDocument();

    resolveTweet({ id: 't1', text: 'late tweet' });

    expect(await screen.findByText('late tweet')).toBeInTheDocument();
  });
});
