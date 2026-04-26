/* SPDX-License-Identifier: AGPL-3.0-or-later */
// @vitest-environment node
// brotli's asm.js encoder pulls a `Browser` global that jsdom doesn't expose;
// the production code runs in a webview where it is defined. Run this spec
// in plain Node so the encoder loads.
import { describe, it, expect } from "vitest";
import { Buffer } from "buffer";
import brotliDecompress from "brotli/decompress";
import { encodeQRPayload, __test } from "@/lib/qr-payload";

const BASE45_ALPHABET =
  "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:";

const BASE45_DECODE = (() => {
  const t = new Int16Array(256).fill(-1);
  for (let i = 0; i < BASE45_ALPHABET.length; i++) {
    t[BASE45_ALPHABET.charCodeAt(i)] = i;
  }
  return t;
})();

function base45Decode(s) {
  if (!s) return new Uint8Array(0);
  const n = s.length;
  const rem = n % 3;
  if (rem === 1) throw new Error("base45 length");
  const full = Math.floor(n / 3);
  const out = new Uint8Array(full * 2 + (rem >> 1));
  let p = 0;
  for (let i = 0; i < full; i++) {
    const c = BASE45_DECODE[s.charCodeAt(i * 3)];
    const d = BASE45_DECODE[s.charCodeAt(i * 3 + 1)];
    const e = BASE45_DECODE[s.charCodeAt(i * 3 + 2)];
    const v = c + d * 45 + e * 2025;
    out[p++] = v >>> 8;
    out[p++] = v & 0xff;
  }
  if (rem === 2) {
    const c = BASE45_DECODE[s.charCodeAt(full * 3)];
    const d = BASE45_DECODE[s.charCodeAt(full * 3 + 1)];
    out[p] = (c + d * 45) & 0xff;
  }
  return out;
}

describe("qr-payload base45", () => {
  it("matches RFC 9285 vectors", () => {
    expect(__test.base45Encode(Buffer.from("AB"))).toBe("BB8");
    expect(__test.base45Encode(Buffer.from("Hello!!"))).toBe("%69 VD92EX0");
    expect(__test.base45Encode(Buffer.from("ietf!"))).toBe("QED8WEX0");
  });
});

describe("encodeQRPayload", () => {
  it("round-trips a realistic AuthNodeInfo", async () => {
    const payload = {
      identity: {
        owner: {
          node_id: "12D3KooWAbcdefghi",
          user_id: "01H0000000000000000000000",
          username: "alice",
          created_at: "2026-01-01T00:00:00Z",
        },
        token: "x".repeat(48),
        psk: "a".repeat(64),
      },
      node_info: {
        node_id: "12D3KooWAbcdefghi",
        owner_id: "01H0000000000000000000000",
        addresses: [
          "/ip4/192.168.1.10/tcp/4001",
          "/ip4/10.0.0.5/tcp/4001",
        ],
        network: "mainnet",
        version: "0.6.250",
      },
    };
    const json = JSON.stringify(payload);
    const encoded = await encodeQRPayload(json);
    expect(encoded.length).toBeGreaterThan(0);
    expect(encoded.length).toBeLessThan(json.length);
    // Output uses only Base45 alphabet characters.
    for (const ch of encoded) {
      expect(BASE45_ALPHABET).toContain(ch);
    }
    // Round-trip via the same brotli decoder used on Android (RFC 7932).
    const compressed = base45Decode(encoded);
    const decompressed = brotliDecompress(compressed);
    expect(Buffer.from(decompressed).toString("utf8")).toBe(json);
  });
});
