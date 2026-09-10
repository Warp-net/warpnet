/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, afterEach } from "vitest";
import { warpnetService, stopRefreshNotifications } from "@/service/service";

const OWNER = {
  user_id: "01H0000000000000000000000",
  node_id: "12D3KooWAbcdefghi",
  network: "testnet",
  username: "alice",
};

const PUBLIC_GET_USER = "/public/get/user/0.0.0";

// The node authenticates once per process and stays that way across page
// reloads. resumeSession decides between "the node still holds my session, go
// back to the app" and "the node was restarted, show the login form" — getting
// it wrong either dead-ends on `already authenticated` or drops the user into
// a dashboard whose every call is refused.
describe("resumeSession", () => {
  afterEach(async () => {
    stopRefreshNotifications();
    try { await warpnetService.logoutUser(); } catch (e) { /* mock */ }
    vi.restoreAllMocks();
  });

  const withOwner = () => warpnetService.setOwnerProfile({ ...OWNER });

  it("returns false without touching the node when no session was restored", async () => {
    const send = vi.spyOn(warpnetService, "sendToNode");

    expect(await warpnetService.resumeSession()).toBe(false);
    expect(send).not.toHaveBeenCalled();
  });

  it("probes the node for the restored owner", async () => {
    withOwner();
    const send = vi
      .spyOn(warpnetService, "sendToNode")
      .mockResolvedValue({ id: OWNER.user_id, username: "alice" });

    await warpnetService.resumeSession();

    expect(send).toHaveBeenCalledWith({
      path: PUBLIC_GET_USER,
      body: { user_id: OWNER.user_id },
    });
  });

  it("resumes when the node answers with the owner", async () => {
    withOwner();
    vi.spyOn(warpnetService, "sendToNode")
      .mockResolvedValue({ id: OWNER.user_id, username: "alice" });

    expect(await warpnetService.resumeSession()).toBe(true);
  });

  it("does not resume when the connection is not signed in", async () => {
    withOwner();
    vi.spyOn(warpnetService, "sendToNode").mockResolvedValue({
      code: 401,
      message: "this connection is not signed in: log in on this node first",
    });

    expect(await warpnetService.resumeSession()).toBe(false);
  });

  it("does not resume on the empty object sendToNode falls back to", async () => {
    withOwner();
    vi.spyOn(warpnetService, "sendToNode").mockResolvedValue({});

    expect(await warpnetService.resumeSession()).toBe(false);
  });

  it("does not resume when the answer is for a different user", async () => {
    withOwner();
    vi.spyOn(warpnetService, "sendToNode")
      .mockResolvedValue({ id: "01H1111111111111111111111" });

    expect(await warpnetService.resumeSession()).toBe(false);
  });

  it("does not resume when the node is unreachable", async () => {
    withOwner();
    vi.spyOn(warpnetService, "sendToNode")
      .mockRejectedValue(new Error("Unable to send"));

    expect(await warpnetService.resumeSession()).toBe(false);
  });
});
