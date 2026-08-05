import { describe, it, expect } from 'vitest';
import {
  isMastodonUser,
  isExperimentalNetwork,
  isMastodonTweet,
  mastodonInstance,
  isOwnTweetEcho,
  decodeHtmlEntities,
} from '@/lib/network';

const ULID = '01ARZ3NDEKTSV4RRFFQ69G5FAV';

describe('isMastodonUser', () => {
  it('classifies a ULID id without a network tag as warpnet', () => {
    expect(isMastodonUser({ id: ULID })).toBe(false);
  });

  it('classifies a lowercase ULID id as warpnet', () => {
    expect(isMastodonUser({ id: ULID.toLowerCase() })).toBe(false);
  });

  it('classifies a fediverse handle without a network tag as mastodon', () => {
    expect(isMastodonUser({ id: 'bob@mastodon.social' })).toBe(true);
  });

  it('prefers the network tag over the id format', () => {
    expect(isMastodonUser({ id: ULID, network: 'mastodon' })).toBe(true);
    expect(isMastodonUser({ id: 'bob@mastodon.social', network: 'warpnet' })).toBe(false);
  });

  it('treats testnet and mainnet tags as warpnet', () => {
    expect(isMastodonUser({ id: 'anything', network: 'testnet' })).toBe(false);
    expect(isMastodonUser({ id: 'anything', network: 'mainnet' })).toBe(false);
  });

  it('handles a missing user', () => {
    expect(isMastodonUser(null)).toBe(false);
  });
});

describe('isMastodonTweet', () => {
  it('classifies by the network tag first', () => {
    expect(isMastodonTweet({ user_id: ULID, network: 'mastodon' })).toBe(true);
    expect(isMastodonTweet({ user_id: 'bob@mastodon.social', network: 'warpnet' })).toBe(false);
    expect(isMastodonTweet({ user_id: 'x', network: 'testnet' })).toBe(false);
    expect(isMastodonTweet({ user_id: 'x', network: 'mainnet' })).toBe(false);
  });

  it('falls back to the user_id shape without a tag', () => {
    expect(isMastodonTweet({ user_id: 'bob@mastodon.social' })).toBe(true);
    expect(isMastodonTweet({ user_id: ULID })).toBe(false);
  });

  it('handles a missing tweet', () => {
    expect(isMastodonTweet(null)).toBe(false);
  });
});

describe('mastodonInstance', () => {
  it('extracts the instance from a fediverse handle', () => {
    expect(mastodonInstance('bob@mastodon.social')).toBe('mastodon.social');
  });

  it('returns empty for non-handles', () => {
    expect(mastodonInstance(ULID)).toBe('');
    expect(mastodonInstance('')).toBe('');
    expect(mastodonInstance(undefined)).toBe('');
    expect(mastodonInstance('@leading-only')).toBe('');
  });
});

describe('decodeHtmlEntities', () => {
  it('decodes the entities the gateway leaves in bridged text', () => {
    expect(decodeHtmlEntities('Linux&#39;s &amp; Unix &quot;systems&quot;'))
      .toBe(`Linux's & Unix "systems"`);
    expect(decodeHtmlEntities('a &lt;tag&gt; stays text')).toBe('a <tag> stays text');
  });

  it('passes plain text through untouched', () => {
    expect(decodeHtmlEntities('no entities here')).toBe('no entities here');
    expect(decodeHtmlEntities('')).toBe('');
    expect(decodeHtmlEntities(undefined)).toBe(undefined);
  });
});

describe('isOwnTweetEcho', () => {
  const OWNER = ULID;

  it('flags the inbound-retweet echo (own ULID boosted by a fediverse handle)', () => {
    expect(isOwnTweetEcho(
      { id: 'tweet1', user_id: OWNER, retweeted_by: 'warpnet@mastodon.social' },
      OWNER,
    )).toBe(true);
  });

  it('flags the fan-out echo (owner as a bridged actor or a gateway status URL)', () => {
    expect(isOwnTweetEcho(
      { id: 'https://gw.ts.net/x', user_id: `${OWNER}@gw.ts.net`, retweeted_by: 'bob@mastodon.social' },
      OWNER,
    )).toBe(true);
    expect(isOwnTweetEcho(
      { id: `https://gw.ts.net/users/${OWNER}/statuses/tweet1`, user_id: 'other', retweeted_by: 'bob@mastodon.social' },
      OWNER,
    )).toBe(true);
  });

  it('keeps retweets by warpnet users and by the owner themselves', () => {
    expect(isOwnTweetEcho(
      { id: 'tweet1', user_id: OWNER, retweeted_by: '01BX5ZZKBKACTAV9WEVGEMMVRY' },
      OWNER,
    )).toBe(false);
    expect(isOwnTweetEcho(
      { id: 'tweet1', user_id: OWNER, retweeted_by: OWNER },
      OWNER,
    )).toBe(false);
  });

  it('keeps foreign boosts and plain tweets', () => {
    expect(isOwnTweetEcho(
      { id: 'https://m.s/1', user_id: 'alice@m.s', retweeted_by: 'bob@mastodon.social' },
      OWNER,
    )).toBe(false);
    expect(isOwnTweetEcho({ id: 'tweet1', user_id: OWNER }, OWNER)).toBe(false);
    expect(isOwnTweetEcho(null, OWNER)).toBe(false);
  });
});

describe('isExperimentalNetwork', () => {
  it('treats the production network as not experimental', () => {
    expect(isExperimentalNetwork('warpnet')).toBe(false);
  });

  it('accepts the unnormalized mainnet alias as production', () => {
    expect(isExperimentalNetwork('mainnet')).toBe(false);
  });

  it('flags the testnet', () => {
    expect(isExperimentalNetwork('testnet')).toBe(true);
  });

  it('flags an unknown network rather than staying silent', () => {
    expect(isExperimentalNetwork('devnet')).toBe(true);
    expect(isExperimentalNetwork('')).toBe(true);
    expect(isExperimentalNetwork(undefined)).toBe(true);
  });

  it('ignores surrounding whitespace and case', () => {
    expect(isExperimentalNetwork('  Warpnet  ')).toBe(false);
  });
});
