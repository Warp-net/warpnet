import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getChats: vi.fn(),
    getDirectMessages: vi.fn(),
    getCursor: vi.fn(),
    setCursor: vi.fn(),
    createChat: vi.fn(),
    sendDirectMessage: vi.fn(),
    markMessageNotificationsRead: vi.fn(),
    markChatRead: vi.fn(),
    getChatReadAt: vi.fn(() => 0),
  },
}));

import Messages from '@/views/Messages.vue';
import { warpnetService } from '@/service/service';

const scrollDirective = {
  mounted() {},
  updated() {},
  unmounted() {},
};

const ALICE = '01KY0357FD1DS8X2E6HHHVXJBG';
const BOB = '01KTRA1Q83VBTES33BRQV79JN6';
const CAROL = '01KSGHBHKG0N77T6A3RZV8WSH5';

const renderMessages = ({ chatId = 'chat-1' } = {}) =>
  render(Messages, {
    global: {
      mocks: {
        $filters: { timeago: () => 'just now', time: () => '12:00' },
        $router: { push: vi.fn() },
        $route: { params: { id: ALICE, chatId } },
      },
      directives: { scroll: scrollDirective, linkify: () => {} },
      stubs: {
        SideNav: true,
        NewMessageOverlay: true,
        ConfirmDialog: true,
        EmojiPicker: true,
        ChatVideo: true,
        Loader: {
          props: ['loading'],
          template: '<div v-if="loading" data-testid="loader" />',
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

beforeEach(() => {
  vi.clearAllMocks();
  warpnetService.getOwnerProfile.mockReturnValue({ user_id: ALICE, username: 'alice' });
  warpnetService.getProfile.mockImplementation(async (id) => ({ id, username: id }));
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.getChats.mockResolvedValue([]);
  warpnetService.getDirectMessages.mockResolvedValue([]);
  warpnetService.getCursor.mockReturnValue('');
  warpnetService.markMessageNotificationsRead.mockResolvedValue(undefined);
});

describe('Messages.vue', () => {
  it('shows the empty state when there are no chats', async () => {
    renderMessages();

    expect(await screen.findByText(/No chats yet/i)).toBeInTheDocument();
  });

  it('renders chats and messages without waiting for a hanging peer profile', async () => {
    warpnetService.getChats.mockResolvedValue([
      { id: 'chat-1', owner_id: ALICE, other_user_id: BOB, last_message: 'hi bob' },
      { id: 'chat-2', owner_id: ALICE, other_user_id: CAROL, last_message: 'hi carol' },
    ]);
    warpnetService.getProfile.mockImplementation((id) =>
      id === CAROL
        ? new Promise(() => {})
        : Promise.resolve({ id, username: id })
    );
    warpnetService.getDirectMessages.mockResolvedValue([
      { id: 'm1', sender_id: BOB, text: 'hello there', created_at: '2026-01-01T10:00:00Z' },
    ]);

    renderMessages({ chatId: 'chat-1' });

    expect(
      await screen.findByText('hello there', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    // Carol's chat row is still listed as a placeholder.
    expect(screen.getByText('hi carol')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByTestId('loader')).not.toBeInTheDocument());
  });

  it('renders message text before its attachments resolve', async () => {
    warpnetService.getChats.mockResolvedValue([
      { id: 'chat-1', owner_id: ALICE, other_user_id: BOB, last_message: '' },
    ]);
    warpnetService.getDirectMessages.mockResolvedValue([
      {
        id: 'm1',
        sender_id: BOB,
        text: 'look at this photo',
        image_keys: ['k1'],
        created_at: '2026-01-01T10:00:00Z',
      },
    ]);
    warpnetService.getImage.mockImplementation(() => new Promise(() => {}));

    renderMessages({ chatId: 'chat-1' });

    expect(
      await screen.findByText('look at this photo', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    expect(screen.queryByAltText('Attachment')).not.toBeInTheDocument();
  });

  it('keeps bridged mastodon chats hidden', async () => {
    warpnetService.getChats.mockResolvedValue([
      { id: 'chat-m', owner_id: ALICE, other_user_id: 'bob@mastodon.social', last_message: 'boo' },
    ]);
    warpnetService.getProfile.mockImplementation(async (id) => ({ id, username: 'bob' }));

    renderMessages({ chatId: 'nope' });

    expect(await screen.findByText(/No chats yet/i)).toBeInTheDocument();
    expect(screen.queryByText('boo')).not.toBeInTheDocument();
  });
});
