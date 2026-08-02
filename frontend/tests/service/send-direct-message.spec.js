import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PUBLIC_POST_MESSAGE } from '@/service/service';
import { Call } from '@/lib/transport';

const sentBody = () => Call.mock.calls[0][0].body;

beforeEach(() => {
  vi.clearAllMocks();
  Call.mockResolvedValue({code: 200, body: {}});
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({
    user_id: 'owner1',
    node_id: 'node-owner',
  });
});

describe('sendDirectMessage attachments', () => {
  it('sends the video key so the recipient can fetch the clip', async () => {
    await warpnetService.sendDirectMessage({
      chatId: 'owner1:friend1',
      receiverId: 'friend1',
      text: '',
      imageKeys: ['poster-key'],
      videoKey: 'video-key',
    });

    expect(Call).toHaveBeenCalledTimes(1);
    expect(Call.mock.calls[0][0].path).toBe(PUBLIC_POST_MESSAGE);
    expect(sentBody()).toMatchObject({
      chat_id: 'owner1:friend1',
      receiver_id: 'friend1',
      sender_id: 'owner1',
      image_keys: ['poster-key'],
      video_key: 'video-key',
    });
  });

  it('sends a whole gallery of image keys, in order', async () => {
    await warpnetService.sendDirectMessage({
      chatId: 'owner1:friend1',
      receiverId: 'friend1',
      text: 'look',
      imageKeys: ['k1', 'k2', 'k3', 'k4'],
    });

    expect(sentBody().image_keys).toEqual(['k1', 'k2', 'k3', 'k4']);
    expect(sentBody()).not.toHaveProperty('video_key');
  });

  it('leaves the attachment keys out of a plain text message', async () => {
    await warpnetService.sendDirectMessage({
      chatId: 'owner1:friend1',
      receiverId: 'friend1',
      text: 'hello',
      imageKeys: [],
    });

    expect(sentBody().text).toBe('hello');
    expect(sentBody()).not.toHaveProperty('image_keys');
    expect(sentBody()).not.toHaveProperty('video_key');
  });
});
