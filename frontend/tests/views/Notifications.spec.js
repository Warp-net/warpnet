import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getNotifications: vi.fn(),
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
          text: 'bob liked your tweet',
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

    expect(await screen.findByText('bob liked your tweet')).toBeInTheDocument();
    expect(screen.getByText('carol followed you')).toBeInTheDocument();
    expect(screen.queryByText(/No notifications yet/i)).not.toBeInTheDocument();
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
          text: 'carol liked your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications();
    await screen.findByText('carol liked your tweet');

    await fireEvent.click(screen.getByRole('button', { name: 'Mentions' }));

    await waitFor(() => {
      expect(screen.getByText('bob mentioned you')).toBeInTheDocument();
      expect(
        screen.queryByText('carol liked your tweet')
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
          text: 'bob liked your tweet',
          created_at: new Date().toISOString(),
        },
      ],
    });

    renderNotifications({ query: { m: 'Mentions' } });

    expect(await screen.findByText(/No mentions yet/i)).toBeInTheDocument();
    expect(screen.queryByText('bob liked your tweet')).not.toBeInTheDocument();
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
