/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
  OpenVideoStream: vi.fn(),
}));

import {
  warpnetService, PRIVATE_GET_VIDEO, PRIVATE_GET_CHAT_VIDEO, VIDEO_STOP_PATH,
} from '@/service/service';
import { Call, OpenVideoStream } from '@/lib/transport';

let close;
beforeEach(() => {
  vi.clearAllMocks();
  close = vi.fn();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
  Call.mockResolvedValue({ body: 'accepted' });
  OpenVideoStream.mockImplementation(async (request) => {
    request.message_id = 'stream-1';
    return { response: { body: { size: 31200683, mime: 'video/mp4' } }, close };
  });
});

describe('video streams', () => {
  it('opens one stream from an offset and hands the frames over as bytes', async () => {
    const frames = [];
    const stream = await warpnetService.openVideoStream({ userId: 'author1', key: 'k1', offset: 1048576 }, (f) => frames.push(f));

    const [request, onFrame] = OpenVideoStream.mock.calls[0];
    expect(request.path).toBe(PRIVATE_GET_VIDEO);
    expect(request.body).toEqual({ user_id: 'author1', key: 'k1', offset: 1048576 });
    expect(stream.size).toBe(31200683);
    expect(stream.mime).toBe('video/mp4');

    onFrame({ offset: 1048576, data: 'AQID', is_last: true });
    expect(frames).toHaveLength(1);
    expect(frames[0].offset).toBe(1048576);
    expect(Array.from(frames[0].data)).toEqual([1, 2, 3]);
    expect(frames[0].isLast).toBe(true);
  });

  it('stops the stream on request', async () => {
    const stream = await warpnetService.openVideoStream({ userId: 'author1', key: 'k1', offset: 0 }, () => {});

    stream.stop();
    expect(close).toHaveBeenCalled();
    expect(Call.mock.calls[0][0].path).toBe(VIDEO_STOP_PATH);
    expect(Call.mock.calls[0][0].body).toEqual({ stream_id: 'stream-1' });
  });

  it('streams chat videos over the chat route', async () => {
    await warpnetService.openVideoStream({ userId: 'sender1', key: 'k2', offset: 0, chat: true }, () => {});

    expect(OpenVideoStream.mock.calls[0][0].path).toBe(PRIVATE_GET_CHAT_VIDEO);
  });

  it('has nothing to play when the node has no such video', async () => {
    OpenVideoStream.mockResolvedValue({ response: { body: { size: 0, mime: '' } }, close });

    expect(await warpnetService.openVideoStream({ userId: 'author1', key: 'gone', offset: 0 }, () => {})).toBeNull();
    expect(close).toHaveBeenCalled();
  });
});
