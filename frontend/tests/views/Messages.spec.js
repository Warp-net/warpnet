import { describe, it, expect, beforeAll, afterAll, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/vue';

vi.mock('@/service/service', () => ({
  warpnetService: {
    getOwnerProfile: vi.fn(),
    getProfile: vi.fn(),
    getImage: vi.fn(),
    getChatImage: vi.fn(),
    getChats: vi.fn(),
    getDirectMessages: vi.fn(),
    getCursor: vi.fn(),
    setCursor: vi.fn(),
    createChat: vi.fn(),
    sendDirectMessage: vi.fn(),
    markMessageNotificationsRead: vi.fn(),
    markChatRead: vi.fn(),
    getChatReadAt: vi.fn(() => 0),
    uploadChatImages: vi.fn(),
    uploadChatVideo: vi.fn(),
  },
}));

vi.mock('@/lib/video', async (importOriginal) => ({
  ...(await importOriginal()),
  captureVideoPoster: vi.fn().mockResolvedValue(null),
}));

import Messages from '@/views/Messages.vue';
import { warpnetService } from '@/service/service';
import { captureVideoPoster } from '@/lib/video';

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
  warpnetService.getChatImage.mockResolvedValue(null);
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
    warpnetService.getChatImage.mockImplementation(() => new Promise(() => {}));

    renderMessages({ chatId: 'chat-1' });

    expect(
      await screen.findByText('look at this photo', undefined, { timeout: 3000 })
    ).toBeInTheDocument();
    expect(screen.queryByAltText('Attachment')).not.toBeInTheDocument();
  });

  it('attaches an image pasted into the message box', async () => {
    warpnetService.getChats.mockResolvedValue([
      { id: 'chat-1', owner_id: ALICE, other_user_id: BOB, last_message: '' },
    ]);
    warpnetService.uploadChatImages.mockResolvedValue(['img-key']);
    renderMessages({ chatId: 'chat-1' });
    const box = await screen.findByPlaceholderText('Start a new message');
    const shot = new File(['png'], 'shot.png', { type: 'image/png' });

    await fireEvent.paste(box, { clipboardData: { files: [shot], types: ['Files'] } });

    await waitFor(() => expect(warpnetService.uploadChatImages).toHaveBeenCalledWith([expect.stringMatching(/^data:image\/png;base64,/)]));
    expect(screen.getByAltText('Image preview')).toBeInTheDocument();
  });

  describe('video attachments', () => {
    const { createObjectURL, revokeObjectURL } = URL;
    beforeAll(() => {
      URL.createObjectURL = vi.fn(() => 'blob:clip');
      URL.revokeObjectURL = vi.fn();
    });
    afterAll(() => {
      URL.createObjectURL = createObjectURL;
      URL.revokeObjectURL = revokeObjectURL;
    });

    const attachClip = async () => {
      warpnetService.getChats.mockResolvedValue([
        { id: 'chat-1', owner_id: ALICE, other_user_id: BOB, last_message: '' },
      ]);
      const { container } = renderMessages({ chatId: 'chat-1' });
      await screen.findByPlaceholderText('Start a new message');
      const input = container.querySelector('input[type="file"][accept*="video/mp4"]');
      const clip = new File(['mp4'], 'clip.mp4', { type: 'video/mp4' });
      Object.defineProperty(input, 'files', { value: [clip], configurable: true });
      await fireEvent.change(input);
      await waitFor(() => expect(captureVideoPoster).toHaveBeenCalled());
    };

    it('uploads the video and its frame only when the message is sent', async () => {
      captureVideoPoster.mockResolvedValueOnce('data:image/jpeg;base64,BBBB');
      warpnetService.uploadChatImages.mockResolvedValue(['poster-key']);
      warpnetService.uploadChatVideo.mockResolvedValue('video-key');
      warpnetService.sendDirectMessage.mockResolvedValue({ id: 'm1' });
      await attachClip();

      expect(warpnetService.uploadChatVideo).not.toHaveBeenCalled();
      expect(warpnetService.uploadChatImages).not.toHaveBeenCalled();
      await fireEvent.click(screen.getByLabelText('Send message'));

      await waitFor(() => expect(warpnetService.sendDirectMessage).toHaveBeenCalledWith(
        expect.objectContaining({ imageKeys: ['poster-key'], videoKey: 'video-key' }),
      ));
      expect(warpnetService.uploadChatVideo).toHaveBeenCalledWith(expect.stringMatching(/^data:video\/mp4;base64,/));
      expect(warpnetService.uploadChatImages).toHaveBeenCalledWith(['data:image/jpeg;base64,BBBB']);
    });

    it('keeps the uploaded video for the retry when the send fails', async () => {
      warpnetService.uploadChatVideo.mockResolvedValue('video-key');
      warpnetService.sendDirectMessage.mockRejectedValueOnce(new Error('node is busy'));
      await attachClip();

      await fireEvent.click(screen.getByLabelText('Send message'));
      await waitFor(() => expect(warpnetService.sendDirectMessage).toHaveBeenCalledTimes(1));
      await waitFor(() => expect(screen.getByLabelText('Send message')).toBeEnabled());

      warpnetService.sendDirectMessage.mockResolvedValue({ id: 'm1' });
      await fireEvent.click(screen.getByLabelText('Send message'));

      await waitFor(() => expect(warpnetService.sendDirectMessage).toHaveBeenCalledTimes(2));
      expect(warpnetService.sendDirectMessage).toHaveBeenLastCalledWith(
        expect.objectContaining({ videoKey: 'video-key' }),
      );
      expect(warpnetService.uploadChatVideo).toHaveBeenCalledTimes(1);
    });
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
