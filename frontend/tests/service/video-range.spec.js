/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_GET_VIDEO, PRIVATE_GET_CHAT_VIDEO } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', node_id: 'node-owner' });
});

describe('video ranges', () => {
  it('reads a range of a timeline video through the node and decodes its bytes', async () => {
    Call.mockResolvedValue({ body: { size: 31200683, mime: 'video/mp4', data: 'AQID' } });

    const range = await warpnetService.getVideoRange({ userId: 'author1', key: 'k1', offset: 1048576, length: 1048576 });

    const request = Call.mock.calls[0][0];
    expect(request.path).toBe(PRIVATE_GET_VIDEO);
    expect(request.body).toEqual({ user_id: 'author1', key: 'k1', offset: 1048576, length: 1048576 });
    expect(range.size).toBe(31200683);
    expect(range.mime).toBe('video/mp4');
    expect(Array.from(range.data)).toEqual([1, 2, 3]);
  });

  it('reads chat videos over the chat route', async () => {
    Call.mockResolvedValue({ body: { size: 3, mime: 'video/mp4', data: 'AQID' } });

    await warpnetService.getChatVideoRange({ userId: 'sender1', key: 'k2', offset: 0, length: 3 });

    expect(Call.mock.calls[0][0].path).toBe(PRIVATE_GET_CHAT_VIDEO);
  });

  it('has nothing to give when the node has no such video', async () => {
    Call.mockResolvedValue({ body: { size: 0, mime: '', data: null } });

    expect(await warpnetService.getVideoRange({ userId: 'author1', key: 'gone', offset: 0, length: 10 })).toBeNull();
  });
});
