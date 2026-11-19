import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getNotifications: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getTweet: vi.fn(),
    getNotificationsPage: vi.fn(),
    createChat: vi.fn(),
    getFollowRequests: vi.fn(),
    markNotificationRead: vi.fn(),
    markAllNotificationsRead: vi.fn(),
    subscribeNotifications: vi.fn(),
  },
}));

import Notifications from '@/views/Notifications.vue';
import { warpnetService } from '@/service/service';

const routerPush = vi.fn();
const routerReplace = vi.fn();

const renderNotifications = ({ query = {} } = {}) =>
  render(Notifications, {
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: routerPush, replace: routerReplace },
        $route: { query },
      },
      stubs: { SideNav: true, DefaultRightBar: true },
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
  routerPush.mockClear();
  routerReplace.mockClear();
  warpnetService.getOwnerProfile.mockReturnValue({
    user_id: 'alice',
    username: 'alice',
  });
  warpnetService.getNotifications.mockResolvedValue({
    unread_count: 0,
    notifications: [],
  });
  warpnetService.getProfile.mockResolvedValue({ user_id: 'alice', locked: false });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getTweet.mockResolvedValue(null);
  warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: [] });
  warpnetService.markNotificationRead.mockResolvedValue({});
  warpnetService.markAllNotificationsRead.mockResolvedValue({});
  warpnetService.subscribeNotifications.mockReturnValue(() => {});
});

describe('Notifications.vue', () => {
  it('renders the Notifications header and tabs', async () => {
    renderNotifications();

    expect(
      await screen.findByRole('heading', { name: 'Notifications' })
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mentions' })).toBeInTheDocument();
  });

  it('shows the empty state when there are no notifications', async () => {
    renderNotifications();

    expect(
      await screen.findByText(/No notifications yet/i)
    ).toBeInTheDocument();
  });

  it('renders loaded notifications with their text', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 2,
      notifications: [
        {
          id: 'n1',
          type: 'like',
          user_id: 'bob',
          text: 'bob reacted your tweet',
          created_at: new Date().toISOString(),
        },
        {
          id: 'n2',
          type: 'follow',
          user_id: 'carol',
          text: 'carol followed you',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();

    expect(await screen.findByText('bob reacted your tweet')).toBeInTheDocument();
    expect(screen.getByText('carol followed you')).toBeInTheDocument();
    expect(screen.queryByText(/No notifications yet/i)).not.toBeInTheDocument();
  });

  it('marks a buyer reaching the order limit with the sponsored pepper', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n3',
          type: 'order_limit',
          user_id: 'author1',
          actor_id: 'buyer1',
          text: 'leaker bought 10 of your tweets within an hour; their next orders wait',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();

    const row = await screen.findByText(/leaker bought 10 of your tweets within an hour/);
    expect(row.closest('[role="button"]').querySelector('.fa-pepper-hot')).not.toBeNull();
  });

  it('opens the tweet when a reaction notification is clicked', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'reaction',
          user_id: 'alice',
          actor_id: 'bob',
          tweet_id: 't1',
          text: 'bob reacted your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();
    await fireEvent.click(await screen.findByText('bob reacted your tweet'));

    expect(routerPush).toHaveBeenCalledWith({
      name: 'Tweet',
      params: { id: 't1' },
      query: { u: 'alice' },
    });
  });

  it('shows the actor and the tweet the action was taken on, each opening its own page', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'reaction',
          user_id: 'alice',
          actor_id: 'bob',
          tweet_id: 't1',
          text: 'bob reacted your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });
    warpnetService.getProfile.mockImplementation((id) =>
      Promise.resolve(id === 'bob' ? { id: 'bob', username: 'bob' } : { user_id: id, locked: false })
    );
    warpnetService.getTweet.mockResolvedValue({ id: 't1', user_id: 'alice', text: 'my first tweet' });

    renderNotifications();

    const tweet = await screen.findByText('my first tweet');
    expect(warpnetService.getTweet).toHaveBeenCalledWith({ userId: 'alice', tweetId: 't1' });
    await fireEvent.click(tweet);
    expect(routerPush).toHaveBeenLastCalledWith({
      name: 'Tweet',
      params: { id: 't1' },
      query: { u: 'alice' },
    });

    await fireEvent.click(screen.getByText('bob', { selector: 'button' }));
    expect(routerPush).toHaveBeenLastCalledWith({ name: 'Profile', params: { id: 'bob' } });
    expect(screen.getByText(/reacted your tweet/)).toBeInTheDocument();
  });

  it('groups reactions to one tweet into a row naming the others', async () => {
    const at = new Date().toISOString();
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 2,
      notifications: [
        { id: 'n2', type: 'reaction', user_id: 'alice', actor_id: 'carol', tweet_id: 't1', text: 'carol reacted your tweet', created_at: at },
        { id: 'n1', type: 'reaction', user_id: 'alice', actor_id: 'bob', tweet_id: 't1', text: 'bob reacted your tweet', created_at: at },
      ],
    });
    warpnetService.getProfile.mockImplementation((id) =>
      Promise.resolve(id === 'alice' ? { user_id: 'alice', locked: false } : { id, username: id })
    );

    renderNotifications();

    expect(await screen.findByText(/and 1 other reacted your tweet/)).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'bob' })).toHaveLength(1);
    expect(screen.getAllByText('carol', { selector: 'button' })).toHaveLength(1);
  });

  it('opens the chat with the sender of a message notification', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        { id: 'n1', type: 'message', user_id: 'alice', actor_id: 'bob', text: 'bob sent you a message', created_at: new Date().toISOString() },
      ],
    });
    warpnetService.createChat.mockResolvedValue({ id: 'chat1' });

    renderNotifications();
    await fireEvent.click(await screen.findByText('bob sent you a message'));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Messages', params: { id: 'alice', chatId: 'chat1' } });
    });
    expect(warpnetService.createChat).toHaveBeenCalledWith('bob');
  });

  it('narrows the list to one notification type from the title menu', async () => {
    const at = new Date().toISOString();
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 2,
      notifications: [
        { id: 'n2', type: 'follow', user_id: 'alice', actor_id: 'carol', text: 'carol started following you', created_at: at },
        { id: 'n1', type: 'reaction', user_id: 'alice', actor_id: 'bob', tweet_id: 't1', text: 'bob reacted your tweet', created_at: at },
      ],
    });

    renderNotifications();
    await screen.findByText('bob reacted your tweet');

    await fireEvent.click(screen.getByRole('button', { name: 'Notification type' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Follows' }));

    await waitFor(() => {
      expect(screen.queryByText('bob reacted your tweet')).not.toBeInTheDocument();
    });
    expect(screen.getByText('carol started following you')).toBeInTheDocument();
  });

  it('loads the next page when the end of the list comes into view', async () => {
    const observers = [];
    vi.stubGlobal('IntersectionObserver', class {
      constructor(cb) { this.cb = cb; observers.push(this); }
      observe() {}
      unobserve() {}
      disconnect() {}
    });
    const at = new Date().toISOString();
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 0,
      cursor: 'c1',
      notifications: [{ id: 'n2', type: 'follow', user_id: 'alice', actor_id: 'carol', text: 'carol started following you', created_at: at }],
    });
    warpnetService.getNotificationsPage.mockResolvedValueOnce({
      cursor: 'end',
      notifications: [{ id: 'n1', type: 'moderation', user_id: 'alice', text: 'your report was reviewed', created_at: at }],
    });

    renderNotifications();
    await screen.findByText('carol started following you');
    observers.forEach(o => o.cb([{ isIntersecting: true }]));

    expect(await screen.findByText('your report was reviewed')).toBeInTheDocument();
    expect(warpnetService.getNotificationsPage).toHaveBeenCalledWith('c1');
    vi.unstubAllGlobals();
  });

  it('opens the follower profile when a follow notification is clicked', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'follow',
          user_id: 'alice',
          actor_id: 'carol',
          text: 'carol started following you',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();
    await fireEvent.click(await screen.findByText('carol started following you'));

    expect(routerPush).toHaveBeenCalledWith({ name: 'Profile', params: { id: 'carol' } });
  });

  it('stays on the page for a notification with neither tweet nor actor', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'moderation',
          user_id: 'alice',
          text: 'your report was reviewed',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();
    await fireEvent.click(await screen.findByText('your report was reviewed'));

    expect(routerPush).not.toHaveBeenCalled();
  });

  it('navigates home when the back button is clicked', async () => {
    renderNotifications();
    await screen.findByRole('heading', { name: 'Notifications' });

    const backButton = screen.getAllByRole('button').find((btn) =>
      btn.querySelector('.fa-arrow-left')
    );
    await fireEvent.click(backButton);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({ name: 'Home' });
    });
  });

  it('switches to the Mentions tab when Mentions is clicked', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'mention',
          user_id: 'bob',
          text: 'bob mentioned you',
          created_at: new Date().toISOString(),
        },
        {
          id: 'n2',
          type: 'like',
          user_id: 'carol',
          text: 'carol reacted your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();
    await screen.findByText('carol reacted your tweet');

    await fireEvent.click(screen.getByRole('button', { name: 'Mentions' }));

    await waitFor(() => {
      expect(screen.getByText('bob mentioned you')).toBeInTheDocument();
      expect(
        screen.queryByText('carol reacted your tweet')
      ).not.toBeInTheDocument();
    });
    expect(routerReplace).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Notifications',
        query: expect.objectContaining({ m: 'Mentions' }),
      })
    );
  });

  it('shows the empty-mentions state when Mentions has no items', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 0,
      notifications: [
        {
          id: 'n1',
          type: 'like',
          user_id: 'bob',
          text: 'bob reacted your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications({ query: { m: 'Mentions' } });

    expect(await screen.findByText(/No mentions yet/i)).toBeInTheDocument();
    expect(screen.queryByText('bob reacted your tweet')).not.toBeInTheDocument();
  });

  it('marks everything read on open with a single node-side call', async () => {
    renderNotifications();
    await screen.findByRole('heading', { name: 'Notifications' });

    await waitFor(() => {
      expect(warpnetService.markAllNotificationsRead).toHaveBeenCalled();
    });
    // The old per-item loop only covered the first page.
    expect(warpnetService.markNotificationRead).not.toHaveBeenCalled();
  });

  it('the settings menu "Mark all as read" calls the node-side read-all', async () => {
    renderNotifications();
    await screen.findByRole('heading', { name: 'Notifications' });
    // Let the on-open auto-read from created() settle before isolating
    // the button's own call.
    await waitFor(() => {
      expect(warpnetService.markAllNotificationsRead).toHaveBeenCalled();
    });
    warpnetService.markAllNotificationsRead.mockClear();

    await fireEvent.click(
      screen.getByRole('button', { name: 'Notification settings' })
    );
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Mark all as read' })
    );

    await waitFor(() => {
      expect(warpnetService.markAllNotificationsRead).toHaveBeenCalledTimes(1);
    });
  });

  it('renders the list without waiting for a hanging actor profile', async () => {
    warpnetService.getNotifications.mockResolvedValueOnce({
      unread_count: 1,
      notifications: [
        {
          id: 'n1',
          type: 'reply',
          actor_id: 'bob',
          text: 'bob replied to you',
          created_at: new Date().toISOString(),
        },
      ],
    });
    warpnetService.getProfile.mockImplementation((id) =>
      id === 'bob' ? new Promise(() => {}) : Promise.resolve({ user_id: id, locked: false })
    );

    renderNotifications();

    expect(
      await screen.findByText('bob replied to you', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
  });

  it('does not pile duplicate profile fetches onto a hanging actor node', async () => {
    let liveCb;
    warpnetService.subscribeNotifications.mockImplementation((cb) => {
      liveCb = cb;
      return () => {};
    });
    const list = [
      {
        id: 'n1',
        type: 'reply',
        actor_id: 'bob',
        text: 'bob replied to you',
        created_at: new Date().toISOString(),
      },
    ];
    warpnetService.getNotifications.mockResolvedValueOnce({ unread_count: 1, notifications: list });
    warpnetService.getProfile.mockImplementation((id) =>
      id === 'bob' ? new Promise(() => {}) : Promise.resolve({ user_id: id, locked: false })
    );

    renderNotifications();
    await screen.findByText('bob replied to you');

    // Two poll deliveries while bob's profile request is still hanging.
    liveCb({ unread_count: 1, notifications: list });
    liveCb({ unread_count: 1, notifications: list });
    await waitFor(() => expect(screen.getByText('bob replied to you')).toBeInTheDocument());

    const bobCalls = warpnetService.getProfile.mock.calls.filter(([id]) => id === 'bob');
    expect(bobCalls).toHaveLength(1);
  });

  it('lists a follow request even when the requester profile hangs', async () => {
    warpnetService.getProfile.mockImplementation((id) =>
      id === 'alice'
        ? Promise.resolve({ user_id: 'alice', locked: true })
        : new Promise(() => {})
    );
    warpnetService.getFollowRequests.mockResolvedValue({ follower_ids: ['bob'] });

    renderNotifications({ query: { m: 'Requests' } });

    expect(await screen.findByText('@bob', undefined, { timeout: 3000 })).toBeInTheDocument();
  });

  it('still renders the header when the backend fails (error state)', async () => {
    warpnetService.getNotifications.mockRejectedValueOnce(new Error('boom'));

    renderNotifications();

    expect(
      await screen.findByRole('heading', { name: 'Notifications' })
    ).toBeInTheDocument();
    expect(
      await screen.findByText(/No notifications yet/i)
    ).toBeInTheDocument();
  });
});
