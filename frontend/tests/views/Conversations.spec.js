import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getChats: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    createChat: vi.fn(),
    markMessageNotificationsRead: vi.fn(),
    getCursor: vi.fn(),
    setCursor: vi.fn(),
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

const renderConversations = ({ id = ALICE } = {}) =>
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
            '<div data-testid="new-message-overlay"><button @click="$emit(\'selected\', { id: CAROL })">pick-user</button></div>',
          data: () => ({ CAROL }),
          emits: ['selected', 'update:showNewMessageModal'],
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

const ALICE = '01KY0357FD1DS8X2E6HHHVXJBG';
const BOB = '01KTRA1Q83VBTES33BRQV79JN6';
const CAROL = '01KSGHBHKG0N77T6A3RZV8WSH5';

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
  warpnetService.markMessageNotificationsRead.mockResolvedValue(undefined);
  warpnetService.getCursor.mockReturnValue('');
});

describe('Conversations.vue', () => {
  it('renders the Chats title', async () => {
    renderConversations();
    expect(await screen.findByText('Chats')).toBeInTheDocument();
  });

  it('shows the empty-state message when there are no chats', async () => {
    renderConversations();
    expect(await screen.findByText(/No messages yet/i)).toBeInTheDocument();
  });

  it('renders a chat list entry with the other user and last message', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-1',
        owner_id: ALICE,
        other_user_id: BOB,
        last_message: 'see you tomorrow',
      },
    ]);

    renderConversations();

    expect(await screen.findByText(BOB)).toBeInTheDocument();
    expect(screen.getByText('see you tomorrow')).toBeInTheDocument();
    expect(screen.queryByText(/No messages yet/i)).not.toBeInTheDocument();
  });

  it('navigates to Messages when a chat row is clicked', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-1',
        owner_id: ALICE,
        other_user_id: BOB,
        last_message: 'hi',
      },
    ]);

    renderConversations();

    const row = await screen.findByText(BOB);
    await fireEvent.click(row);

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: ALICE, chatId: 'chat-1' },
      });
    });
  });

  it('opens the new message overlay when the "New message" button is clicked', async () => {
    renderConversations();
    await screen.findByText(/No messages yet/i);

    const newMsgBtn = screen.getByRole('button', { name: 'New message' });
    await fireEvent.click(newMsgBtn);

    expect(await screen.findByTestId('new-message-overlay')).toBeInTheDocument();
  });

  it('creates a new chat and navigates when selecting a user without an existing chat', async () => {
    renderConversations();
    await screen.findByText(/No messages yet/i);

    await fireEvent.click(screen.getByRole('button', { name: 'New message' }));
    await fireEvent.click(await screen.findByText('pick-user'));

    await waitFor(() => {
      expect(warpnetService.createChat).toHaveBeenCalledWith(CAROL);
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: ALICE, chatId: 'new-chat-id' },
      });
    });
  });

  it('reuses the existing chat when selecting a user already in the list', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-existing',
        owner_id: ALICE,
        other_user_id: CAROL,
        last_message: '',
      },
    ]);

    renderConversations();
    await screen.findByText(CAROL);

    await fireEvent.click(screen.getByRole('button', { name: 'New message' }));
    await fireEvent.click(await screen.findByText('pick-user'));

    await waitFor(() => {
      expect(routerPush).toHaveBeenCalledWith({
        name: 'Messages',
        params: { id: ALICE, chatId: 'chat-existing' },
      });
    });
    expect(warpnetService.createChat).not.toHaveBeenCalled();
  });

  it('still lists a chat whose other user could not be resolved', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-unresolved',
        owner_id: ALICE,
        other_user_id: BOB,
        last_message: 'ping',
      },
    ]);
    warpnetService.getProfile.mockImplementation(async (id) =>
      id === BOB ? { code: 5000, message: 'get user: other user user not found' } : { id, username: id }
    );

    renderConversations();

    expect(await screen.findByText('ping')).toBeInTheDocument();
    expect(screen.getByText('Anonymous')).toBeInTheDocument();
    expect(screen.queryByText(/No messages yet/i)).not.toBeInTheDocument();
  });

  it('keeps the chat row when the avatar fetch fails', async () => {
    warpnetService.getChats.mockResolvedValue([
      {
        id: 'chat-avatar',
        owner_id: ALICE,
        other_user_id: BOB,
        last_message: 'yo',
      },
    ]);
    warpnetService.getProfile.mockImplementation(async (id) => ({
      id,
      username: id,
      avatar_key: 'some-key',
    }));
    warpnetService.getImage.mockRejectedValue(new Error('ERR_TIMEOUT'));

    renderConversations();

    expect(await screen.findByText(BOB)).toBeInTheDocument();
    expect(screen.getByText('yo')).toBeInTheDocument();
  });

  it('clears the loader when the chat list request fails', async () => {
    warpnetService.getChats.mockRejectedValue(new Error('node unreachable'));

    renderConversations();

    expect(await screen.findByText(/No messages yet/i)).toBeInTheDocument();
  });
});
