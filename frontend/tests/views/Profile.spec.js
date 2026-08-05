import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getUsers: vi.fn(),
    getTweets: vi.fn(),
    getReplies: vi.fn(),
    getReactions: vi.fn(),
    getFollowers: vi.fn(),
    getFollowings: vi.fn(),
    isFollowing: vi.fn(),
    isFollower: vi.fn(),
    isUserBlocked: vi.fn(),
    isUserMuted: vi.fn(),
  },
}));

import Profile from '@/views/Profile.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = { mounted() {}, updated() {}, unmounted() {} };
const linkifyDirective = { mounted() {}, updated() {}, unmounted() {} };

const tweetsStub = {
  props: ['tweets'],
  template: '<div><article v-for="t in tweets" :key="t.id">{{ t.text }}</article></div>',
};

const renderProfile = () =>
  render(Profile, {
    global: {
      mocks: {
        $router: { push: vi.fn() },
        $route: { params: { id: 'alice' } },
      },
      directives: { scroll: scrollDirective, linkify: linkifyDirective },
      stubs: {
        SideNav: true,
        DefaultRightBar: true,
        Loader: true,
        ConfirmDialog: true,
        ReportDialog: true,
        EditProfileOverlay: true,
        SetUpProfileOverlay: true,
        Tweets: tweetsStub,
      },
    },
  });

const textTweet = { id: 't1', user_id: 'alice', text: 'plain tweet', created_at: '2026-01-01T10:00:00Z' };
const photoTweet = {
  id: 't2',
  user_id: 'alice',
  text: 'tweet with a photo',
  image_keys: ['img-1'],
  created_at: '2026-01-02T10:00:00Z',
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
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'alice', username: 'Alice' });
  warpnetService.getProfile.mockResolvedValue({
    id: 'alice',
    user_id: 'alice',
    username: 'Alice',
    created_at: '2025-12-01T10:00:00Z',
  });
  warpnetService.getImage.mockResolvedValue('');
  warpnetService.getUsers.mockResolvedValue([]);
  warpnetService.getFollowers.mockResolvedValue([]);
  warpnetService.getFollowings.mockResolvedValue([]);
  warpnetService.isFollowing.mockResolvedValue(false);
  warpnetService.isFollower.mockResolvedValue(false);
  warpnetService.isUserBlocked.mockResolvedValue(false);
  warpnetService.isUserMuted.mockResolvedValue(false);
  warpnetService.getReplies.mockResolvedValue([]);
  warpnetService.getReactions.mockResolvedValue({ items: [] });
  warpnetService.getTweets.mockImplementation(({ cursorReset }) =>
    Promise.resolve(cursorReset ? [photoTweet, textTweet] : [])
  );
});

describe('Profile.vue first paint under hanging elements', () => {
  it('renders the tweets even when avatar and background blobs hang', async () => {
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));

    renderProfile();

    expect(
      await screen.findByText('plain tweet', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
  });

  it('renders the tweets while the followers list hangs', async () => {
    warpnetService.getFollowers.mockImplementation(() => new Promise(() => {}));
    warpnetService.getFollowings.mockImplementation(() => new Promise(() => {}));

    renderProfile();

    expect(
      await screen.findByText('plain tweet', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
  });

  it('shows the profile header even when the tweets request hangs', async () => {
    warpnetService.getTweets.mockImplementation(() => new Promise(() => {}));

    renderProfile();

    expect(
      await screen.findByRole('heading', { name: 'Alice' })
    ).toBeInTheDocument();
  });
});

describe('Profile.vue tabs', () => {
  it('keeps only tweets carrying a photo or a video in the media tab', async () => {
    renderProfile();
    await waitFor(() => expect(screen.getByText('plain tweet')).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: 'Media' }));

    await waitFor(() => expect(screen.queryByText('plain tweet')).not.toBeInTheDocument());
    expect(screen.getByText('tweet with a photo')).toBeInTheDocument();
  });

  it('pages further into the profile when the first page holds no media', async () => {
    const videoTweet = {
      id: 't3',
      user_id: 'alice',
      text: 'tweet with a video',
      video_key: 'vid-1',
      created_at: '2026-01-03T10:00:00Z',
    };
    warpnetService.getTweets.mockImplementation(({ cursorReset }) =>
      Promise.resolve(cursorReset ? [textTweet] : [videoTweet])
    );

    renderProfile();
    await waitFor(() => expect(screen.getByText('plain tweet')).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: 'Media' }));

    await waitFor(() => expect(screen.getByText('tweet with a video')).toBeInTheDocument());
  });

  it('shows the owner replies from their own threads next to their tweets', async () => {
    warpnetService.getReplies.mockImplementation(({ parentId }) =>
      Promise.resolve(
        parentId === 't1'
          ? [
              { id: 'r1', user_id: 'alice', parent_id: 't1', text: 'my own follow-up', created_at: '2026-01-01T11:00:00Z' },
              { id: 'r2', user_id: 'bob', parent_id: 't1', text: 'someone else answer', created_at: '2026-01-01T12:00:00Z' },
            ]
          : []
      )
    );

    renderProfile();
    await waitFor(() => expect(screen.getByText('plain tweet')).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: 'Tweets and threads' }));

    await waitFor(() => expect(screen.getByText('my own follow-up')).toBeInTheDocument());
    expect(screen.getByText('plain tweet')).toBeInTheDocument();
    expect(screen.queryByText('someone else answer')).not.toBeInTheDocument();
  });

  it('leaves the tweets tab free of replies', async () => {
    warpnetService.getReplies.mockResolvedValue([
      { id: 'r1', user_id: 'alice', parent_id: 't1', text: 'my own follow-up', created_at: '2026-01-01T11:00:00Z' },
    ]);

    renderProfile();
    await waitFor(() => expect(screen.getByText('plain tweet')).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: 'Tweets and threads' }));
    await waitFor(() => expect(screen.getByText('my own follow-up')).toBeInTheDocument());

    await fireEvent.click(screen.getByRole('button', { name: 'Tweets' }));

    await waitFor(() => expect(screen.queryByText('my own follow-up')).not.toBeInTheDocument());
    expect(screen.getByText('plain tweet')).toBeInTheDocument();
  });
});
