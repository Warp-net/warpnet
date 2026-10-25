/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PRIVATE_POST_TWEET, PRIVATE_POST_SPONSORED_TWEET, PRIVATE_POST_SETTINGS_SPONSORED } from '@/service/service';
import { Call } from '@/lib/transport';

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue({ user_id: 'owner1', username: 'Owner', node_id: 'node-owner' });
});

describe('createTweet with a price', () => {
  it('goes to the sponsored route and carries the price', async () => {
    Call.mockResolvedValue({ body: { id: 't1' } });

    await warpnetService.createTweet({ text: 'members only', price: '1500000' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_SPONSORED_TWEET);
    expect(sent.body.price).toEqual({ units: 1500000 });
  });

  it('leaves a free tweet on the plain route', async () => {
    Call.mockResolvedValue({ body: { id: 't1' } });

    await warpnetService.createTweet({ text: 'hello' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_TWEET);
    expect(sent.body).not.toHaveProperty('price');
  });

  // No node serves the route yet, and its refusal must not read as posted.
  it.each([
    ['an error body', { body: { code: 500, message: 'protocols not supported' } }],
    ['no body at all', { code: 500, message: 'protocols not supported' }],
  ])('rejects when the node answers with %s', async (_, answer) => {
    Call.mockResolvedValue(answer);

    await expect(warpnetService.createTweet({ text: 'members only', price: '1500000' }))
      .rejects.toThrow("can't publish sponsored tweets");
  });
});

describe('getSponsoredTweet', () => {
  it('reads an order the network has not confirmed yet as pending', async () => {
    Call.mockResolvedValue({ body: { tweet_id: 't1', tx_id: 'tx1', confirmed: false } });

    expect(await warpnetService.getSponsoredTweet({ tweetId: 't1', userId: 'author1' })).toEqual({ pending: true });
  });

  it('hands over a bought tweet and nothing for an unbought one', async () => {
    Call.mockResolvedValueOnce({ body: { id: 't1', text: 'paid words' } }).mockResolvedValueOnce({ body: { id: '' } });

    expect(await warpnetService.getSponsoredTweet({ tweetId: 't1', userId: 'author1' })).toEqual({ id: 't1', text: 'paid words' });
    expect(await warpnetService.getSponsoredTweet({ tweetId: 't1', userId: 'author1' })).toBeNull();
  });
});

describe('createTweet at the price ceiling', () => {
  it('sends 1 000 000 USDT as exact base units', async () => {
    Call.mockResolvedValue({ body: { id: 't1' } });

    await warpnetService.createTweet({ text: 'members only', price: '1000000000000' });

    expect(Call.mock.calls[0][0].body.price).toEqual({ units: 1000000000000 });
    expect(JSON.stringify(Call.mock.calls[0][0].body.price)).toBe('{"units":1000000000000}');
  });
});

describe('sponsored settings', () => {
  it('sends the stake as whole TRX and hands back what the node saved', async () => {
    Call.mockResolvedValue({ body: { min_stake_trx: 500 } });

    expect(await warpnetService.updateSponsoredSettings('500.7')).toEqual({ min_stake_trx: 500 });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PRIVATE_POST_SETTINGS_SPONSORED);
    expect(sent.body).toEqual({ min_stake_trx: 500 });
  });

  it.each([['-3'], [''], ['abc']])('sends %j as no stake', async (input) => {
    Call.mockResolvedValue({ body: { min_stake_trx: 0 } });

    await warpnetService.updateSponsoredSettings(input);

    expect(Call.mock.calls[0][0].body).toEqual({ min_stake_trx: 0 });
  });

  it('rejects a node that did not save', async () => {
    Call.mockResolvedValue({ body: { code: 500, message: 'update sponsored settings: empty owner' } });

    await expect(warpnetService.updateSponsoredSettings(500)).rejects.toThrow('empty owner');
  });
});
