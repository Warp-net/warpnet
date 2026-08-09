import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getReactions: vi.fn(),
    getTweet: vi.fn(),
    getOwnerProfile: vi.fn(),
  },
}));

import Reactions from '@/views/Reactions.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const renderReactions = () =>
  render(Reactions, {
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
  warpnetService.getReactions.mockResolvedValue({ items: [], cursor: 'end' });
  warpnetService.getTweet.mockResolvedValue(null);
});

describe('Reactions.vue', () => {
  it('shows the empty state when nothing was reacted', async () => {
    renderReactions();
    expect(await screen.findByText(/Nothing reacted yet/i)).toBeInTheDocument();
  });

  it('renders a reacted tweet once hydrated', async () => {
    warpnetService.getReactions.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    warpnetService.getTweet.mockResolvedValue({ id: 't1', text: 'liked tweet' });

    renderReactions();

    expect(await screen.findByText('liked tweet')).toBeInTheDocument();
    expect(warpnetService.getTweet).toHaveBeenCalledWith({ userId: 'u1', tweetId: 't1' });
  });

  it('clears the loader even when tweet hydration hangs', async () => {
    warpnetService.getReactions.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    warpnetService.getTweet.mockImplementation(() => new Promise(() => {}));

    renderReactions();

    await waitFor(
      () => expect(screen.queryByTestId('loader')).not.toBeInTheDocument(),
      { timeout: 3000 },
    );
  });

  it('fills the tweet in place when hydration resolves late', async () => {
    warpnetService.getReactions.mockResolvedValue({
      items: [{ tweet_id: 't1', owner_user_id: 'u1' }],
      cursor: 'end',
    });
    let resolveTweet;
    warpnetService.getTweet.mockImplementation(
      () => new Promise((resolve) => { resolveTweet = resolve; }),
    );

    renderReactions();

    await waitFor(
      () => expect(screen.queryByTestId('loader')).not.toBeInTheDocument(),
      { timeout: 3000 },
    );
    expect(screen.queryByText('late tweet')).not.toBeInTheDocument();

    resolveTweet({ id: 't1', text: 'late tweet' });

    expect(await screen.findByText('late tweet')).toBeInTheDocument();
  });

  it('a hanging sibling does not block the other tweet from rendering', async () => {
    warpnetService.getReactions.mockResolvedValue({
      items: [
        { tweet_id: 't1', owner_user_id: 'u1' },
        { tweet_id: 't2', owner_user_id: 'u2' },
      ],
      cursor: 'end',
    });
    warpnetService.getTweet.mockImplementation(({ tweetId }) =>
      tweetId === 't1'
        ? Promise.resolve({ id: 't1', text: 'fast tweet' })
        : new Promise(() => {})
    );

    renderReactions();

    expect(await screen.findByText('fast tweet')).toBeInTheDocument();
    expect(screen.queryByText(/Nothing reacted yet/i)).not.toBeInTheDocument();
  });
});
