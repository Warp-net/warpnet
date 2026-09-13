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
  it('shows the network and the instance on a Mastodon tweet', async () => {
    const { getByTitle, getByText, getByLabelText } = renderTweet({ ...bridgedTweet });
    await waitFor(() => {
      expect(getByTitle('Bridged from Mastodon — mastodon.social')).toBeTruthy();
    });
    expect(getByText('mastodon.social')).toBeTruthy();
    expect(getByLabelText('Mastodon')).toBeTruthy();
  });

  it('shows the Threads mark on a Threads tweet', async () => {
    const { getByTitle, getByText, getByLabelText } = renderTweet({
      ...bridgedTweet,
      id: 'https://threads.net/ap/users/17841452547050663/post/1/',
      user_id: 'someone@threads.net',
      username: 'someone@threads.net',
      network: 'threads',
      root_id: 'https://threads.net/ap/users/17841452547050663/post/1/',
    });
    await waitFor(() => {
      expect(getByTitle('Bridged from Threads — threads.net')).toBeTruthy();
    });
    expect(getByText('threads.net')).toBeTruthy();
    expect(getByLabelText('Threads')).toBeTruthy();
  });

  it('offers no reply on a Threads tweet', async () => {
    const { getByTitle, getByLabelText } = renderTweet({
      ...bridgedTweet,
      user_id: 'someone@threads.net',
      network: 'threads',
    });
    await waitFor(() => {
      expect(getByTitle('Threads posts cannot be replied to from Warpnet')).toBeTruthy();
    });
    expect(getByLabelText('Reply').disabled).toBe(true);
  });

  it('still offers a reply on a Mastodon tweet', async () => {
    const { getByLabelText } = renderTweet({ ...bridgedTweet });
    await waitFor(() => expect(getByLabelText('Reply')).toBeTruthy());
    expect(getByLabelText('Reply').disabled).toBe(false);
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

  it('decodes html entities in bridged text but not in warpnet text', async () => {
    const { getByText } = renderTweet({
      ...bridgedTweet,
      text: 'Linux&#39;s kernel &amp; more',
    });
    await waitFor(() => expect(getByText(/Linux's kernel & more/)).toBeTruthy());

    const { getByText: getWarpnet } = renderTweet({
      ...warpnetTweet,
      text: 'literal &#39; stays',
    });
    await waitFor(() => expect(getWarpnet(/literal &#39; stays/)).toBeTruthy());
  });
});
