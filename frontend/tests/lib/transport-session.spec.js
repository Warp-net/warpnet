/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, beforeEach } from "vitest";

const CLIENT_KEY = "warpnet.client.key";

vi.mock("@/lib/noise", async () => {
  const actual = await vi.importActual("@/lib/noise");
  class FakeInitiator {
    constructor(staticPriv) {
      this.staticPriv = staticPriv;
    }
    writeMessageA() {
      return new Uint8Array([0x01]);
    }
    readMessageB() {}
    writeMessageC() {
      return {
        message: new TextEncoder().encode("static:" + this.staticPriv),
        session: {
          remoteStatic: new Uint8Array([0x02]),
          encrypt: (plain) => plain,
          decrypt: (frame) => frame,
        },
      };
    }
  }
  return {
    ...actual,
    NoiseInitiator: FakeInitiator,
    fingerprint: () => "a".repeat(64),
  };
});

let sockets = [];

class FakeWebSocket {
  constructor(url) {
    this.url = url;
    this.readyState = 1; // OPEN
    this.sent = [];
    sockets.push(this);
  }
  send(data) {
    this.sent.push(data);
  }
  close() {
    this.readyState = 3;
  }
}
FakeWebSocket.OPEN = 1;

const text = (frame) => new TextDecoder().decode(frame);
const flush = async () => {
  for (let i = 0; i < 3; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
};

async function loadTransport() {
  vi.resetModules();
  sockets = [];
  globalThis.WebSocket = FakeWebSocket;
  delete window.go; // stay on the browser bridge, not the Wails binding
  return import("@/lib/transport");
}

async function openSocket() {
  await flush();
  const sock = sockets[0];
  sock.onopen();
  sock.onmessage({ data: new Uint8Array([0x03]).buffer });
  await flush();
  return sock;
}

describe("transport client identity", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("presents its static key before sending any request", async () => {
    const transport = await loadTransport();

    const call = transport.Call({ path: "/private/get/user/0.0.0", body: {} });
    const sock = await openSocket();

    expect(sock.sent).toHaveLength(3);
    expect(text(sock.sent[1])).toContain("static:");
    expect(JSON.parse(text(sock.sent[2])).path).toBe("/private/get/user/0.0.0");

    const req = JSON.parse(text(sock.sent[2]));
    sock.onmessage({
      data: new TextEncoder().encode(
        JSON.stringify({ message_id: req.message_id, body: { user_id: "u1" } })
      ).buffer,
    });
    await expect(call).resolves.toMatchObject({ body: { user_id: "u1" } });
  });

  it("keeps the same key across reloads, so one login enrolls this browser for good", async () => {
    await loadTransport();
    (await import("@/lib/noise")).loadOrCreateClientKey();
    const first = localStorage.getItem(CLIENT_KEY);
    expect(first).toMatch(/^[0-9a-f]{64}$/);

    await loadTransport();
    (await import("@/lib/noise")).loadOrCreateClientKey();
    expect(localStorage.getItem(CLIENT_KEY)).toBe(first);
  });
});

describe("transport video stream", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("routes pushed frames to the stream and the answer to the request", async () => {
    const transport = await loadTransport();
    const frames = [];

    const opened = transport.OpenVideoStream({ path: "/private/get/video/0.0.0", body: {} }, (f) => frames.push(f));
    const sock = await openSocket();
    const req = JSON.parse(text(sock.sent[sock.sent.length - 1]));

    const push = (msg) => sock.onmessage({ data: new TextEncoder().encode(JSON.stringify(msg)).buffer });
    push({ message_id: req.message_id, path: "video-frame", body: { offset: 0, data: "AQID" } });
    push({ message_id: req.message_id, path: "/private/get/video/0.0.0", body: { size: 3, mime: "video/mp4" } });

    const { response, close } = await opened;
    expect(response.body).toEqual({ size: 3, mime: "video/mp4" });
    expect(frames).toEqual([{ offset: 0, data: "AQID" }]);

    close();
    push({ message_id: req.message_id, path: "video-frame", body: { offset: 3 } });
    expect(frames).toHaveLength(1);
  });

  it("tells open streams when the connection drops", async () => {
    const transport = await loadTransport();
    const frames = [];

    transport.OpenVideoStream({ path: "/private/get/video/0.0.0", body: {} }, (f) => frames.push(f)).catch(() => {});
    const sock = await openSocket();
    sock.onclose();

    expect(frames).toEqual([{ error: "connection to the node was lost" }]);
  });
});
