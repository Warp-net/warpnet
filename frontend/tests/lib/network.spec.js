import { describe, it, expect } from 'vitest';
import { isMastodonUser } from '@/lib/network';

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
