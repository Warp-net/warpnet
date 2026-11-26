/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PUBLIC_GET_WHOTOFOLLOW } from '@/service/service';
import { Call } from '@/lib/transport';

const owner = { user_id: 'owner1', username: 'Owner', node_id: 'node-owner' };
const answer = (body) => Call.mockResolvedValueOnce({ body });
const allUsers = (pages) => Object.values(pages).flatMap((p) => p.users);

let errSpy;
beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue(owner);
  errSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  Call.mockResolvedValue({ body: { networks: {} } });
});
afterEach(() => {
  errSpy.mockRestore();
});

describe('getWhoToFollow', () => {
  it('asks the owner-scoped route with the cursors and limit it was given', async () => {
    await warpnetService.getWhoToFollow({ mastodon: 'm1', threads: 't1' }, 7);

    const [req] = Call.mock.calls[0];
    expect(req.path).toBe(PUBLIC_GET_WHOTOFOLLOW);
    expect(req.body).toEqual({ limit: 7, cursors: { mastodon: 'm1', threads: 't1' }, user_id: 'owner1' });
  });

  it('asks a first page with no cursors and the default limit', async () => {
    await warpnetService.getWhoToFollow();

    expect(Call.mock.calls[0][0].body).toEqual({ limit: 20, cursors: {}, user_id: 'owner1' });
  });

  it('never sends the legacy flat cursor', async () => {
    await warpnetService.getWhoToFollow({ warpnet: 'w1' });

    expect(Call.mock.calls[0][0].body).not.toHaveProperty('cursor');
  });

  it.each([
    ['no owner', undefined],
    ['an owner without an id', { username: 'ghost' }],
    ['an owner with an empty id', { user_id: '' }],
  ])('does not call the node with %s', async (_, profile) => {
    warpnetService.getOwnerProfile.mockReturnValue(profile);

    await expect(warpnetService.getWhoToFollow()).resolves.toEqual({});
    expect(Call).not.toHaveBeenCalled();
  });

  it.each([
    ['a node error with no body', { code: 500, message: 'boom' }],
    ['an empty body', { body: {} }],
    ['a null networks map', { body: { networks: null } }],
    ['a string networks map', { body: { networks: 'warpnet' } }],
    ['a numeric networks map', { body: { networks: 42 } }],
    ['a legacy flat answer', { body: { users: [{ id: 'u1' }], cursor: 'c' } }],
  ])('answers no networks for %s', async (_, reply) => {
    Call.mockResolvedValueOnce(reply);

    await expect(warpnetService.getWhoToFollow()).resolves.toEqual({});
  });

  it('rejects when the node does not answer at all', async () => {
    Call.mockResolvedValueOnce(null);

    await expect(warpnetService.getWhoToFollow()).rejects.toThrow();
  });

  it('rejects when the transport throws', async () => {
    Call.mockRejectedValueOnce(new Error('socket closed'));

    await expect(warpnetService.getWhoToFollow()).rejects.toThrow('socket closed');
  });

  it('turns an array in place of the map into no users', async () => {
    answer({ networks: [{ users: [{ id: 'u1' }] }] });

    const pages = await warpnetService.getWhoToFollow();

    expect(allUsers(pages).map((u) => u.id)).toEqual(['u1']);
    expect(Object.values(pages).every((p) => p.cursor === null)).toBe(true);
  });

  it('maps the end and a missing cursor to null and keeps a real one', async () => {
    answer({
      networks: {
        warpnet: { users: [], cursor: 'end' },
        mastodon: { users: [], cursor: '' },
        threads: { users: [] },
        nostr: { users: [], cursor: null },
        bluesky: { users: [], cursor: '/USERS/USER/None/500/x' },
      },
    });

    const pages = await warpnetService.getWhoToFollow();

    expect(pages.warpnet.cursor).toBeNull();
    expect(pages.mastodon.cursor).toBeNull();
    expect(pages.threads.cursor).toBeNull();
    expect(pages.nostr.cursor).toBeNull();
    expect(pages.bluesky.cursor).toBe('/USERS/USER/None/500/x');
  });

  it('survives broken pages without dropping the good ones', async () => {
    answer({
      networks: {
        warpnet: null,
        mastodon: { cursor: 'm2' },
        threads: { users: null, cursor: 'end' },
        nostr: { users: [{ id: 'n1' }], cursor: 'end' },
      },
    });

    const pages = await warpnetService.getWhoToFollow();

    expect(pages.warpnet).toEqual({ users: [], cursor: null });
    expect(pages.mastodon).toEqual({ users: [], cursor: 'm2' });
    expect(pages.threads).toEqual({ users: [], cursor: null });
    expect(pages.nostr.users.map((u) => u.id)).toEqual(['n1']);
  });

  it('drops offline accounts but keeps their network cursor', async () => {
    answer({
      networks: {
        mastodon: {
          users: [
            { id: 'on', isOffline: false },
            { id: 'off', isOffline: true },
            { id: 'unknown' },
          ],
          cursor: 'm2',
        },
        threads: { users: [{ id: 'gone', isOffline: true }], cursor: 't2' },
      },
    });

    const pages = await warpnetService.getWhoToFollow();

    expect(pages.mastodon.users.map((u) => u.id)).toEqual(['on', 'unknown']);
    expect(pages.threads).toEqual({ users: [], cursor: 't2' });
  });

  it('keeps the network keys exactly as the node sent them', async () => {
    answer({ networks: { '': { users: [{ id: 'legacy' }] }, Mastodon: { users: [] }, 'сеть': { users: [] } } });

    const pages = await warpnetService.getWhoToFollow();

    expect(Object.keys(pages).sort()).toEqual(['', 'Mastodon', 'сеть'].sort());
  });

  it('does not share in-flight requests between callers', async () => {
    await Promise.all([warpnetService.getWhoToFollow({ mastodon: 'a' }), warpnetService.getWhoToFollow({ threads: 'b' })]);

    expect(Call).toHaveBeenCalledTimes(2);
    expect(Call.mock.calls.map(([req]) => req.message_id)).toHaveLength(2);
    expect(new Set(Call.mock.calls.map(([req]) => req.message_id)).size).toBe(2);
  });
});
