import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

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
    isBookmarked: vi.fn(),
    deleteReply: vi.fn(),
  },
}));

import TweetBlock from '@/components/TweetBlock.vue';
import { warpnetService } from '@/service/service';

class FakeIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const reply = (over = {}) => ({
  id: 'r1',
  user_id: 'stranger',
  username: 'stranger',
  text: 'unwanted reply',
  created_at: '2026-10-16T10:00:00Z',
  parent_id: 't1',
  parent_user_id: 'owner1',
  root_id: 't1',
  ...over,
});

const renderReply = (tweet, ownRootId) =>
  render(TweetBlock, {
    props: { tweet, ownRootId },
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
      },
      directives: { linkify: {} },
    },
  });

const openMenu = async () => {
  await fireEvent.click(await screen.findByRole('button', { name: 'Tweet options' }));
};

let logSpy, errSpy, warnSpy;
beforeAll(() => {
  logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
});
afterAll(() => {
  logSpy.mockRestore();
  errSpy.mockRestore();
  warnSpy.mockRestore();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getProfile.mockResolvedValue({ id: 'stranger', username: 'stranger', avatar_key: '' });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
  warpnetService.getTweetStats.mockResolvedValue({ tweet_id: 'r1' });
  warpnetService.hasReactor.mockResolvedValue(false);
  warpnetService.getReactorEmoji.mockResolvedValue('');
  warpnetService.hasRetweeter.mockResolvedValue(false);
  warpnetService.isBookmarked.mockResolvedValue(false);
  warpnetService.deleteReply.mockResolvedValue({});
});

describe('TweetBlock reply in the owner\'s thread', () => {
  it('lets the owner delete someone else\'s reply in their own thread', async () => {
    renderReply(reply(), 't1');

    await openMenu();
    await fireEvent.click(screen.getByRole('button', { name: 'Delete reply' }));
    expect(await screen.findByText('Delete reply?')).toBeInTheDocument();
    expect(screen.getByText(/removed from your thread/)).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => expect(warpnetService.deleteReply).toHaveBeenCalledWith({
      userId: 'stranger',
      parentId: 't1',
      rootId: 't1',
      replyId: 'r1',
    }));
    await waitFor(() => expect(screen.queryByText('unwanted reply')).not.toBeInTheDocument());
  });

  it('offers no delete in someone else\'s thread', async () => {
    renderReply(reply(), '');

    await openMenu();
    expect(screen.queryByRole('button', { name: 'Delete reply' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete tweet' })).not.toBeInTheDocument();
  });

  it('offers no delete for a reply to someone else, which is not stored on the owner\'s node', async () => {
    renderReply(reply({ parent_id: 'r0', parent_user_id: 'bob' }), 't1');

    await openMenu();
    expect(screen.queryByRole('button', { name: 'Delete reply' })).not.toBeInTheDocument();
  });

  it('offers no delete for a reply that names another root, which the node would refuse', async () => {
    renderReply(reply({ parent_id: 'r0', root_id: 'r0' }), 't1');

    await openMenu();
    expect(screen.queryByRole('button', { name: 'Delete reply' })).not.toBeInTheDocument();
  });

  it('keeps the author menu on the owner\'s own reply', async () => {
    renderReply(reply({ user_id: 'owner1', username: 'owner' }), 't1');

    await openMenu();
    expect(screen.getByRole('button', { name: 'Delete tweet' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete reply' })).not.toBeInTheDocument();
  });
});
