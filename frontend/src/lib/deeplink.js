// SPDX-License-Identifier: AGPL-3.0-or-later
// JS mirror of cmd/node/member/deeplink: parse warpnet:// URLs forwarded by the Go side.

export const DEEP_LINK_SCHEME = 'warpnet';
export const DEEP_LINK_KIND_USER = 'user';

/**
 * @param {string|null|undefined} raw
 * @returns {{kind: string, id: string, raw: string}|null}
 */
export function parseDeepLink(raw) {
  if (raw == null) return null;
  const trimmed = String(raw).trim();
  if (!trimmed) return null;

  // Canonicalise "warpnet:user/x" -> "warpnet://user/x".
  const lower = trimmed.toLowerCase();
  let normalised = trimmed;
  if (lower.startsWith(DEEP_LINK_SCHEME + ':') &&
      !lower.startsWith(DEEP_LINK_SCHEME + '://')) {
    normalised = DEEP_LINK_SCHEME + '://' + trimmed.slice(DEEP_LINK_SCHEME.length + 1);
  }

  let u;
  try {
    u = new URL(normalised);
  } catch (_) {
    return null;
  }
  if (u.protocol.toLowerCase() !== DEEP_LINK_SCHEME + ':') return null;

  const kind = u.hostname.toLowerCase();
  switch (kind) {
    case DEEP_LINK_KIND_USER: {
      const id = u.pathname.replace(/^\/+/, '').replace(/\/+$/, '');
      if (!id || id === '.') return null;
      try {
        return { kind, id: decodeURIComponent(id), raw: trimmed };
      } catch (_) {
        return { kind, id, raw: trimmed };
      }
    }
    default:
      return null;
  }
}
