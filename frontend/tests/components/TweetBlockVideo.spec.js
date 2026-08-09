import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, waitFor, fireEvent } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getVideo: vi.fn(),
    getOwnerProfile: vi.fn(),
    getTweetStats: vi.fn(),
    hasReactor: vi.fn(),
    hasRetweeter: vi.fn(),
    viewTweet: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

class FakeIntersectionObserver {
  constructor(callback) {
    this.callback = callback;
  }
  observe() {}
  unobserve() {}
  disconnect() {}
}

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
  warpnetService.getProfile.mockResolvedValue({id: 'author1', username: 'author', avatar_key: ''});
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getVideo.mockResolvedValue({
    file: 'data:video/mp4;base64,AAAA',
    size: 1234,
    deferred: false,
  });
  warpnetService.getOwnerProfile.mockReturnValue({user_id: 'viewer1', node_id: 'node-viewer'});
  warpnetService.getTweetStats.mockResolvedValue({
    tweet_id: 't1', tweets_count: 0, retweets_count: 0,
    reactions_count: 0, replies_count: 0, views_count: 0,
  });
  warpnetService.hasReactor.mockResolvedValue(false);
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.viewTweet.mockResolvedValue(7);
});

const videoTweet = {
  id: 't1',
  user_id: 'author1',
  username: 'author',
  text: 'watch this',
  created_at: '2026-05-04T00:00:00Z',
  parent_id: '',
  root_id: '',
  retweeted_by: '',
  image_keys: [],
  video_key: 'vkey1',
};

const renderTweet = (tweet, props = {}) =>
  render(TweetBlock, {
    props: { tweet, ...props },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
    },
  });

describe('TweetBlock video', () => {
  it('does not fetch the video payload when rendered in a feed', async () => {
    const {getByLabelText} = renderTweet(videoTweet);

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    expect(warpnetService.getVideo).not.toHaveBeenCalled();
    expect(getByLabelText('Play video')).toBeTruthy();
  });

  it('fetches and shows the player only after the user presses play', async () => {
    const {getByLabelText, container} = renderTweet(videoTweet);

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    expect(container.querySelector('video')).toBeNull();

    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => expect(container.querySelector('video')).not.toBeNull());
    expect(warpnetService.getVideo).toHaveBeenCalledWith({
      userId: 'author1',
      key: 'vkey1',
    });
    expect(container.querySelector('video').getAttribute('src'))
      .toBe('data:video/mp4;base64,AAAA');
  });

  it('autoloads the video when autoloadVideo is set', async () => {
    const {container} = renderTweet(videoTweet, {autoloadVideo: true});

    await waitFor(() => expect(warpnetService.getVideo).toHaveBeenCalled());
    await waitFor(() => expect(container.querySelector('video')).not.toBeNull());
  });

  it('shows a message when the author node returns no payload', async () => {
    warpnetService.getVideo.mockResolvedValue({file: '', size: 0, deferred: false});
    const {getByLabelText, getByRole} = renderTweet(videoTweet);

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => {
      expect(getByRole('alert').textContent).toContain("isn't available");
    });
  });

  it('explains a codec failure when the video element errors', async () => {
    const {getByLabelText, container, getByRole} = renderTweet(videoTweet);

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    await fireEvent.click(getByLabelText('Play video'));
    await waitFor(() => expect(container.querySelector('video')).not.toBeNull());

    await fireEvent(container.querySelector('video'), new Event('error'));

    await waitFor(() => {
      expect(getByRole('alert').textContent).toContain('codec');
    });
  });

  it('shows the captured frame on the placeholder instead of as an attachment', async () => {
    warpnetService.getImage.mockImplementation(({key}) =>
      Promise.resolve(key ? `data:image/jpeg;base64,${key}` : null));
    const {container, getByLabelText} = renderTweet({...videoTweet, image_keys: ['poster1']});

    await waitFor(() => {
      expect(getByLabelText('Play video').querySelector('img')).not.toBeNull();
    });
    expect(getByLabelText('Play video').querySelector('img').getAttribute('src'))
      .toBe('data:image/jpeg;base64,poster1');
    expect(container.querySelector('img[alt="Tweet image"]')).toBeNull();
    expect(warpnetService.getVideo).not.toHaveBeenCalled();
  });

  it('hands the captured frame to the player as its poster', async () => {
    warpnetService.getImage.mockImplementation(({key}) =>
      Promise.resolve(key ? `data:image/jpeg;base64,${key}` : null));
    const {container, getByLabelText} = renderTweet({...videoTweet, image_keys: ['poster1']});

    await waitFor(() => {
      expect(getByLabelText('Play video').querySelector('img')).not.toBeNull();
    });
    await fireEvent.click(getByLabelText('Play video'));

    await waitFor(() => {
      expect(container.querySelector('video')?.getAttribute('poster'))
        .toBe('data:image/jpeg;base64,poster1');
    });
  });

  it('still renders attachments as a gallery on a post without a video', async () => {
    warpnetService.getImage.mockImplementation(({key}) =>
      Promise.resolve(key ? `data:image/jpeg;base64,${key}` : null));
    const {container} = renderTweet({
      ...videoTweet,
      video_key: undefined,
      image_keys: ['img1', 'img2'],
    });

    await waitFor(() => {
      expect(container.querySelectorAll('img[alt="Tweet image"]').length).toBe(2);
    });
  });

  it('renders no video block for a tweet without a video key', async () => {
    const {container} = renderTweet({...videoTweet, video_key: undefined});

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    expect(container.querySelector('video')).toBeNull();
    expect(warpnetService.getVideo).not.toHaveBeenCalled();
  });
});

describe('TweetBlock YouTube preview', () => {
  it('shows a click-to-play facade without contacting YouTube', async () => {
    const {container, findByLabelText} = renderTweet({
      ...videoTweet,
      video_key: undefined,
      text: 'see https://youtu.be/dQw4w9WgXcQ',
    });

    await findByLabelText('Play YouTube video dQw4w9WgXcQ');
    expect(container.querySelector('iframe')).toBeNull();
  });

  it('mounts a nocookie iframe after pressing play', async () => {
    const {container, findByLabelText} = renderTweet({
      ...videoTweet,
      video_key: undefined,
      text: 'see https://youtu.be/dQw4w9WgXcQ',
    });

    await fireEvent.click(await findByLabelText('Play YouTube video dQw4w9WgXcQ'));

    await waitFor(() => expect(container.querySelector('iframe')).not.toBeNull());
    expect(container.querySelector('iframe').getAttribute('src'))
      .toContain('youtube-nocookie.com/embed/dQw4w9WgXcQ');
  });

  it('suppresses the YouTube card when the post has its own video', async () => {
    const {container, getByLabelText} = renderTweet({
      ...videoTweet,
      text: 'see https://youtu.be/dQw4w9WgXcQ',
    });

    await waitFor(() => expect(warpnetService.getProfile).toHaveBeenCalled());
    expect(getByLabelText('Play video')).toBeTruthy();
    expect(container.querySelector('iframe')).toBeNull();
  });
});
