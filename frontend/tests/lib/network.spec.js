import { describe, it, expect } from 'vitest';
import {
  isMastodonUser,
  isExperimentalNetwork,
  isMastodonTweet,
  mastodonInstance,
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
