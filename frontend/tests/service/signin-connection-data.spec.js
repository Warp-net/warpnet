/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, afterEach } from "vitest";
import { warpnetService, stopRefreshNotifications } from "@/service/service";
import { encodeQRPayload } from "@/lib/qr-payload";

const AUTH = {
  user_id: "01H0000000000000000000000",
  token: "t".repeat(48),
  psk: "p".repeat(64),
  node_id: "12D3KooWAbcdefghi",
  addresses: ["/ip4/127.0.0.1/tcp/4001"],
  bootstrap_peers: [],
  network: "testnet",
};

describe("signInUser connection-data payload", () => {
  afterEach(async () => {
    stopRefreshNotifications();
    try { await warpnetService.logoutUser(); } catch (e) { /* mock */ }
    vi.restoreAllMocks();
  });

  it("stores the plain AuthNodeInfo JSON for copying, not the compressed QR string", async () => {
    // The login RPC returns the AuthNodeInfo; getProfile back-fills the username.
    vi.spyOn(warpnetService, "sendToNode").mockResolvedValue({ ...AUTH });
    vi.spyOn(warpnetService, "getProfile").mockResolvedValue({ username: "alice" });

    await warpnetService.signInUser({ username: "alice", password: "secret" });

    const stored = warpnetService.getQRPayload();

    // Readable JSON — uncompressed and unencoded (the user's requirement).
    const parsed = JSON.parse(stored);
    expect(parsed.node_id).toBe(AUTH.node_id);
    expect(parsed.user_id).toBe(AUTH.user_id);
    // Still carries the pairing token the QR embeds.
    expect(parsed.token).toBe(AUTH.token);
    // And it is NOT the gzip+Base45 form the QR image itself encodes.
    const encoded = await encodeQRPayload(JSON.stringify(AUTH));
    expect(stored).not.toBe(encoded);
  });
});
