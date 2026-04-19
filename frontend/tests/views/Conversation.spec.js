import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getChat: vi.fn(),
    getDirectMessages: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    sendDirectMessage: vi.fn(),
  },
}));

import Conversation from '@/views/Conversation.vue';
import { warpnetService } from '@/service/service';

const renderConversation = ({ id = 'alice', chatId = 'chat-1' } = {}) =>
  render(Conversation, {
    global: {
      mocks: {
        $filters: { timeago: () => 'just now' },
        $router: { push: vi.fn() },
        $route: { params: { id, chatId } },
      },
      directives: { linkify: () => {} },
      stubs: {
        SideNav: true,
        Loader: true,
        'router-link': { template: '<a><slot /></a>' },
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
  warpnetService.getChat.mockResolvedValue({
    id: 'chat-1',
    owner_id: 'alice',
    other_user_id: 'bob',
  });
  warpnetService.getDirectMessages.mockResolvedValue([]);
  warpnetService.getProfile.mockResolvedValue({
    id: 'bob',
    username: 'bob',
    avatar_key: '',
  });
  warpnetService.getImage.mockResolvedValue(null);
  warpnetService.sendDirectMessage.mockResolvedValue({
    id: 'm-new',
    text: 'hello bob',
    sender_id: 'alice',
    created_at: new Date().toISOString(),
  });
});

describe('Conversation.vue', () => {
  it('renders the other user header after the chat loads', async () => {
    renderConversation();

    expect(await screen.findByText('bob')).toBeInTheDocument();
    expect(screen.getByText('@bob')).toBeInTheDocument();
  });

  it('renders existing messages from the backend', async () => {
    warpnetService.getDirectMessages.mockResolvedValue([
      {
        id: 'm1',
        text: 'hi there',
        sender_id: 'alice',
        created_at: new Date().toISOString(),
      },
      {
        id: 'm2',
        text: 'how are you',
        sender_id: 'bob',
        created_at: new Date().toISOString(),
      },
    ]);

    renderConversation();

    expect(await screen.findByText('hi there')).toBeInTheDocument();
    expect(screen.getByText('how are you')).toBeInTheDocument();
  });

  it('appends a new message when the user sends one', async () => {
    renderConversation();
    // Wait for full load so otherUser and chat are both ready.
    await screen.findByText('bob');

    const input = screen.getByPlaceholderText(/Type a message/i);
    await fireEvent.update(input, 'hello bob');

    await fireEvent.click(screen.getByRole('button', { name: /^Send$/ }));

    await waitFor(() => {
      expect(screen.getByText('hello bob')).toBeInTheDocument();
    });
    expect(warpnetService.sendDirectMessage).toHaveBeenCalledWith({
      chatId: 'chat-1',
      receiverId: 'bob',
      text: 'hello bob',
    });
    expect(input).toHaveValue('');
  });

  it('does not call the backend when submitting whitespace-only input (edge case)', async () => {
    renderConversation();
    await screen.findByText('bob');

    const input = screen.getByPlaceholderText(/Type a message/i);
    await fireEvent.update(input, '   ');
    await fireEvent.click(screen.getByRole('button', { name: /^Send$/ }));

    // Flush microtasks so sendMessage would have had a chance to run.
    await Promise.resolve();
    expect(warpnetService.sendDirectMessage).not.toHaveBeenCalled();
  });

  it('does not append a message if sending fails (error state)', async () => {
    warpnetService.sendDirectMessage.mockRejectedValueOnce(new Error('boom'));
    renderConversation();
    await screen.findByText('bob');

    const input = screen.getByPlaceholderText(/Type a message/i);
    await fireEvent.update(input, 'never sent');
    await fireEvent.click(screen.getByRole('button', { name: /^Send$/ }));

    await waitFor(() => {
      expect(warpnetService.sendDirectMessage).toHaveBeenCalled();
    });
    expect(screen.queryByText('never sent')).not.toBeInTheDocument();
    expect(input).toHaveValue('never sent');
  });
});
