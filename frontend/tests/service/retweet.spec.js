/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, beforeEach, vi } from 'vitest';

vi.mock('@/lib/transport', () => ({
  Call: vi.fn(),
  ConsumePendingDeepLink: vi.fn(),
  IsFirstRun: vi.fn(() => false),
  IsDesktop: vi.fn(() => false),
}));

import { warpnetService, PUBLIC_POST_RETWEET, PUBLIC_POST_UNRETWEET } from '@/service/service';
import { Call } from '@/lib/transport';

const owner = { user_id: 'owner1', username: 'Owner', node_id: 'node-owner' };
let seq = 0;
const freshId = () => `tweet-${++seq}`;

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  vi.spyOn(warpnetService, 'getOwnerProfile').mockReturnValue(owner);
  Call.mockResolvedValue({ body: {} });
});

describe('retweeter cache', () => {
  it('remembers a retweet by the tweet id', async () => {
    const id = freshId();
    await warpnetService.setRetweeter(id, owner.user_id, owner);

    expect(await warpnetService.hasRetweeter(id, owner.user_id)).toBe(true);
    expect(await warpnetService.getRetweeter(id, owner.user_id)).toEqual(owner);
    expect(localStorage.getItem(`retweeter::${id}::${owner.user_id}`)).toBe('1');
  });

  it('treats the RT: card and the original card as one tweet', async () => {
    const id = freshId();
    await warpnetService.setRetweeter(id, owner.user_id, owner);

    expect(await warpnetService.hasRetweeter(`RT:${id}`, owner.user_id)).toBe(true);
    expect(await warpnetService.getRetweeter(`RT:${id}`, owner.user_id)).toEqual(owner);
  });

  it('clears the original card when the retweet is undone from the RT: card', async () => {
    const id = freshId();
    await warpnetService.setRetweeter(id, owner.user_id, owner);

    await warpnetService.deleteRetweeter(`RT:${id}`, owner.user_id);

    expect(await warpnetService.hasRetweeter(id, owner.user_id)).toBe(false);
    expect(await warpnetService.getRetweeter(id, owner.user_id)).toBeUndefined();
    expect(localStorage.getItem(`retweeter::${id}::${owner.user_id}`)).toBeNull();
  });

  it('stores a retweet made from the RT: card under the original id', async () => {
    const id = freshId();
    await warpnetService.setRetweeter(`RT:${id}`, owner.user_id, owner);

    expect(localStorage.getItem(`retweeter::${id}::${owner.user_id}`)).toBe('1');
    expect(localStorage.getItem(`retweeter::RT:${id}::${owner.user_id}`)).toBeNull();
  });

  it('reads a retweet persisted by an earlier session', async () => {
    const id = freshId();
    localStorage.setItem(`retweeter::${id}::${owner.user_id}`, '1');

    expect(await warpnetService.hasRetweeter(id, owner.user_id)).toBe(true);
    expect(await warpnetService.hasRetweeter(`RT:${id}`, owner.user_id)).toBe(true);
  });

  it('keeps retweeters apart', async () => {
    const id = freshId();
    await warpnetService.setRetweeter(id, owner.user_id, owner);

    expect(await warpnetService.hasRetweeter(id, 'someone-else')).toBe(false);
  });

  it('strips the prefix only from the start of the id', async () => {
    const id = `x${freshId()}RT:`;
    await warpnetService.setRetweeter(id, owner.user_id, owner);

    expect(localStorage.getItem(`retweeter::${id}::${owner.user_id}`)).toBe('1');
    expect(await warpnetService.hasRetweeter(id.replace('RT:', ''), owner.user_id)).toBe(false);
  });
});

describe('retweetTweet', () => {
  it('sends a plain retweet as is', async () => {
    await warpnetService.retweetTweet({ tweetId: 't1', userId: 'author1', username: 'author', text: 'hi' });

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PUBLIC_POST_RETWEET);
    expect(sent.body).toMatchObject({
      id: 't1',
      user_id: 'author1',
      username: 'author',
      text: 'hi',
      retweeted_by: owner.user_id,
    });
  });

  it('retweets the original when sent from the RT: card', async () => {
    await warpnetService.retweetTweet({ tweetId: 'RT:t1', userId: owner.user_id, username: 'Owner', text: 'mine' });

    expect(Call.mock.calls[0][0].body.id).toBe('t1');
  });

  it('quotes the original when sent from the RT: card', async () => {
    await warpnetService.retweetTweet({ tweetId: 'RT:t1', userId: owner.user_id, username: 'Owner', text: 'mine', comment: ' look ' });

    const body = Call.mock.calls[0][0].body;
    expect(body.id).toBe('t1');
    expect(body.quoted_tweet_id).toBe('t1');
    expect(body.quoted_user_id).toBe(owner.user_id);
    expect(body.user_id).toBe(owner.user_id);
    expect(body.text).toBe('look');
  });

  it('treats a blank comment as a plain retweet', async () => {
    await warpnetService.retweetTweet({ tweetId: 't1', userId: 'author1', username: 'author', text: 'hi', comment: '   ' });

    const body = Call.mock.calls[0][0].body;
    expect(body.user_id).toBe('author1');
    expect(body).not.toHaveProperty('quoted_tweet_id');
  });
});

describe('unretweetTweet', () => {
  it('asks the node to undo the owner’s retweet', async () => {
    await warpnetService.unretweetTweet('t1');

    const sent = Call.mock.calls[0][0];
    expect(sent.path).toBe(PUBLIC_POST_UNRETWEET);
    expect(sent.body).toEqual({ retweeter_id: owner.user_id, tweet_id: 't1' });
  });
});
