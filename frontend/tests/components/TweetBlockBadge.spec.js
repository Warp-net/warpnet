import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    hasReactor: vi.fn(),
    getReactorEmoji: vi.fn(),
    hasRetweeter: vi.fn(),
    viewTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

class FakeIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const renderTweet = (tweet) =>
  render(TweetBlock, {
    props: { tweet },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
    },
  });

let logSpy, errSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getProfile.mockResolvedValue({
    id: 'bob@mastodon.social',
    username: 'bob',
    avatar_key: '',
  });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({
    user_id: 'viewer1',
    node_id: 'node-viewer',
  });
  warpnetService.getTweetStats.mockResolvedValue({});
  warpnetService.hasReactor.mockResolvedValue(false);
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.viewTweet.mockResolvedValue(0);
});

const bridgedTweet = {
  id: 'https://mastodon.social/users/bob/statuses/1',
  user_id: 'bob@mastodon.social',
  username: 'bob@mastodon.social',
  text: 'toot',
  created_at: '2026-05-04T00:00:00Z',
  network: 'mastodon',
  parent_id: '',
  root_id: 'https://mastodon.social/users/bob/statuses/1',
  retweeted_by: '',
  image_keys: [],
};

const warpnetTweet = {
  id: 't1',
  user_id: '01ARZ3NDEKTSV4RRFFQ69G5FAV',
  username: 'author',
  text: 'hello world',
  created_at: '2026-05-04T00:00:00Z',
  network: 'warpnet',
  parent_id: '',
  root_id: '',
  retweeted_by: '',
  image_keys: [],
};

describe('TweetBlock bridged badge', () => {
  it('shows the instance badge on a Mastodon tweet', async () => {
    const { getByText } = renderTweet({ ...bridgedTweet });
    await waitFor(() => {
      const badge = getByText('mastodon.social');
      expect(badge.getAttribute('title')).toBe('Bridged from mastodon.social');
    });
  });

  it('shows no badge on a Warpnet tweet', async () => {
    const { queryByTitle } = renderTweet({ ...warpnetTweet });
    expect(queryByTitle(/Bridged from/)).toBeNull();
  });

  it('falls back to a generic label when the id carries no instance', async () => {
    const { getByText } = renderTweet({
      ...bridgedTweet,
      user_id: 'opaque-remote-id',
      username: 'someone',
    });
    await waitFor(() => expect(getByText('Mastodon')).toBeTruthy());
  });
});
