// SPDX-License-Identifier: AGPL-3.0-or-later
// Warpnet — Decentralized Social Network.
//
// JS-side mirror of the Go cmd/node/member/deeplink package: take a
// raw warpnet://… URL handed up from the desktop binary (either
// captured at cold-start from os.Args or, on macOS, from
// Mac.OnUrlOpen) and turn it into a small typed object the UI can
// route on.
//
// The Vue app has no URL router for profiles — it navigates by
// pushing the search box. That's why the kind→action mapping lives
// outside this module: parse() just gives you {kind, id}, the
// caller decides what to do (today: route to Search?q=id).

export const DEEP_LINK_SCHEME = 'warpnet';
export const DEEP_LINK_KIND_USER = 'user';

/**
 * Parse a raw warpnet:// URL.
 * @param {string|null|undefined} raw
 * @returns {{kind: string, id: string, raw: string}|null}
 *   null on anything that isn't a recognized deep link.
 */
export function parseDeepLink(raw) {
  if (raw == null) return null;
  const trimmed = String(raw).trim();
  if (!trimmed) return null;

  // Tolerate the "warpnet:user/x" form some shells produce after
  // stripping the //, matching the Go parser.
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
  // URL keeps the trailing colon on protocol.
  if (u.protocol.toLowerCase() !== DEEP_LINK_SCHEME + ':') return null;

  const kind = u.hostname.toLowerCase();
  switch (kind) {
    case DEEP_LINK_KIND_USER: {
      // pathname is "/<id>"; strip leading + trailing slashes,
      // reject blanks (warpnet://user/) and lone dots.
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
