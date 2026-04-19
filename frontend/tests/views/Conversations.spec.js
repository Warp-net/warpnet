import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getChats: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    createChat: vi.fn(),
  },
}));

import Conversations from '@/views/Conversations.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const routerPush = vi.fn();

const renderConversations = ({ id = 'alice' } = {}) =>
  render(Conversations, {
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: routerPush },
        $route: { params: { id } },
      },
      directives: { scroll: scrollDirective, linkify: () => {} },
      stubs: {
        SideNav: true,
        Loader: true,
        NewMessageOverlay: {
          template:
            '<div data-testid="new-message-overlay"><button @click="$emit(\'selected\', { id: \'carol\' })">pick-user</button></div>',
          emits: ['selected', 'update:showNewMessageModal'],
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
  routerPush.mockClear();
  warpnetService.getChats.mockResolvedValue([]);
  warpnetService.getProfile.mockImplementation(async (id) => ({
    id,
    username: id,
    avatar_key: '',
  }));
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.createChat.mockResolvedValue({ id: 'new-chat-id' });
});

describe('Conversations.vue', () => {
  it('renders the Chats title', async () => {
    renderConversations();
    expect(await screen.findByText('Chats')).toBeInTheDocument();
  });

  it('shows the empty-state message when there are no chats', async () => {
    renderConversations();
    expect(await screen.findByText(/No chats yet/i)).toBeInTheDocument();
  });

  it('renders a chat list entry with the other user and last message', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-1',
        owner_id: 'alice',
        other_user_id: 'bob',
        last_message: 'see you tomorrow',
      },
    ]);

    renderConversations();

    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(screen.getByText('see you tomorrow')).toBeInTheDocument();
    expect(screen.queryByText(/No chats yet/i)).not.toBeInTheDocument();
  });

  it('navigates to Messages when a chat row is clicked', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-1',
        owner_id: 'alice',
        other_user_id: 'bob',
        last_message: 'hi',
      },
    ]);

    renderConversations();

    const row = await screen.findByText('bob');
    await fireEvent.click(row);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: 'alice', chatId: 'chat-1' },
      });
    });
  });

  it('opens the new message overlay when the "New message" button is clicked', async () => {
    renderConversations();
    await screen.findByText(/No chats yet/i);

    const newMsgBtn = screen.getByRole('button', { name: /New message/i });
    await fireEvent.click(newMsgBtn);

    expect(await screen.findByTestId('new-message-overlay')).toBeInTheDocument();
  });

  it('creates a new chat and navigates when selecting a user without an existing chat', async () => {
    renderConversations();
    await screen.findByText(/No chats yet/i);

    await fireEvent.click(screen.getByRole('button', { name: /New message/i }));
    await fireEvent.click(await screen.findByText('pick-user'));

    await waitFor(() => {
      expect(warpnetService.createChat).toHaveBeenCalledWith('carol');
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: 'alice', chatId: 'new-chat-id' },
      });
    });
  });

  it('reuses the existing chat when selecting a user already in the list', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-existing',
        owner_id: 'alice',
        other_user_id: 'carol',
        last_message: '',
      },
    ]);

    renderConversations();
    await screen.findByText('carol');

    await fireEvent.click(screen.getByRole('button', { name: /New message/i }));
    await fireEvent.click(await screen.findByText('pick-user'));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: 'alice', chatId: 'chat-existing' },
      });
    });
    expect(warpnetService.createChat).not.toHaveBeenCalled();
  });
});
