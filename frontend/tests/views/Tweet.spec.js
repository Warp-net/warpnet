import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';
import { nextTick } from 'vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getTweet: vi.fn(),
    getReply: vi.fn(),
    getReplies: vi.fn(),
    replyTweet: vi.fn(),
    isDataSaverEnabled: vi.fn(),
  },
}));

import Tweet from '@/views/Tweet.vue';
import { warpnetService } from '@/service/service';

const renderTweet = ({ params = { id: 't1' }, query = {} } = {}) =>
  render(Tweet, {
    global: {
      mocks: {
        $router: { back: vi.fn(), push: vi.fn() },
        $route: { params, query },
      },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        EmojiPicker: true,
        Loader: {
          props: ['loading'],
          template: '<div v-if="loading" data-testid="loader" />',
        },
        TweetBlock: {
          props: ['tweet', 'ownRootId'],
          template: '<div :data-own-root="ownRootId">{{ tweet.text }}</div>',
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
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'me' });
  warpnetService.getTweet.mockResolvedValue({
    id: 't1',
    user_id: 'author1',
    text: 'root tweet',
  });
  warpnetService.getReplies.mockResolvedValue([]);
  warpnetService.isDataSaverEnabled.mockReturnValue(true);
});

describe('Tweet.vue', () => {
  it('renders the fetched tweet', async () => {
    renderTweet();

    expect(await screen.findByText('root tweet')).toBeInTheDocument();
  });

  it('shows the not-found state when the tweet cannot be fetched', async () => {
    warpnetService.getTweet.mockResolvedValue(null);

    renderTweet();

    expect(await screen.findByText(/Tweet not found/i)).toBeInTheDocument();
  });

  it('a hanging replies request does not block the tweet render', async () => {
    warpnetService.getReplies.mockImplementation(() => new Promise(() => {}));

    renderTweet();

    expect(
      await screen.findByText('root tweet', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByTestId('loader')).not.toBeInTheDocument());
  });

  it('fills the replies in when they arrive late', async () => {
    let resolveReplies;
    warpnetService.getReplies.mockImplementation(
      () => new Promise((resolve) => { resolveReplies = resolve; }),
    );

    renderTweet();
    await screen.findByText('root tweet');
    expect(screen.queryByText('late reply')).not.toBeInTheDocument();

    resolveReplies([{ id: 'r1', user_id: 'bob', text: 'late reply' }]);

    expect(await screen.findByText('late reply')).toBeInTheDocument();
  });
});

describe('Tweet.vue thread owner', () => {
  const replyPage = { params: { id: 'r1' }, query: { u: 'bob', parent: 't1', root: 't1' } };

  beforeEach(() => {
    warpnetService.getReply.mockResolvedValue({
      id: 'r1', user_id: 'bob', text: 'a reply', parent_id: 't1', root_id: 't1',
    });
  });

  it('marks the replies under the owner\'s own tweet as their thread', async () => {
    warpnetService.getTweet.mockResolvedValue({ id: 't1', user_id: 'me', text: 'my tweet' });
    warpnetService.getReplies.mockResolvedValue([{ id: 'r1', user_id: 'bob', text: 'a reply' }]);

    renderTweet();

    const reply = await screen.findByText('a reply');
    await waitFor(() => expect(reply).toHaveAttribute('data-own-root', 't1'));
    expect(warpnetService.getTweet).toHaveBeenCalledTimes(1);
  });

  it('does not mark the replies under someone else\'s tweet', async () => {
    warpnetService.getReplies.mockResolvedValue([{ id: 'r1', user_id: 'bob', text: 'a reply' }]);

    renderTweet();

    expect(await screen.findByText('a reply')).toHaveAttribute('data-own-root', '');
    expect(warpnetService.getTweet).toHaveBeenCalledTimes(1);
  });

  it('finds the root among the owner\'s tweets on a reply page', async () => {
    warpnetService.getTweet.mockResolvedValue({ id: 't1', user_id: 'me', text: 'my tweet' });
    warpnetService.getReplies.mockResolvedValue([{ id: 'r2', user_id: 'carol', text: 'nested reply' }]);

    renderTweet(replyPage);

    const nested = await screen.findByText('nested reply');
    await waitFor(() => expect(nested).toHaveAttribute('data-own-root', 't1'));
    expect(screen.getByText('a reply')).toHaveAttribute('data-own-root', 't1');
    expect(warpnetService.getTweet).toHaveBeenCalledWith({ userId: 'me', tweetId: 't1' });
  });

  it('does not mark a reply page whose root is not the owner\'s', async () => {
    warpnetService.getTweet.mockResolvedValue({ code: 404, message: 'tweet not found' });
    warpnetService.getReplies.mockResolvedValue([{ id: 'r2', user_id: 'carol', text: 'nested reply' }]);

    renderTweet(replyPage);

    const nested = await screen.findByText('nested reply');
    await waitFor(() => expect(warpnetService.getTweet).toHaveBeenCalled());
    await warpnetService.getTweet.mock.results[0].value;
    await nextTick();
    expect(nested).toHaveAttribute('data-own-root', '');
  });
});
