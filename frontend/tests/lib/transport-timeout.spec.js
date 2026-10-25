/* SPDX-License-Identifier: AGPL-3.0-or-later */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/noise", async () => {
  const actual = await vi.importActual("@/lib/noise");
  class FakeInitiator {
    writeMessageA() {
      return new Uint8Array([0x01]);
    }
    readMessageB() {}
    writeMessageC() {
      return {
        message: new Uint8Array([0x02]),
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
    this.readyState = 1;
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

const flush = async () => {
  for (let i = 0; i < 3; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
};

const answer = (sock, frame, body) => {
  const req = JSON.parse(new TextDecoder().decode(frame));
  sock.onmessage({
    data: new TextEncoder().encode(JSON.stringify({ message_id: req.message_id, body })).buffer,
  });
};

async function connectedTransport() {
  vi.resetModules();
  sockets = [];
  globalThis.WebSocket = FakeWebSocket;
  delete window.go;
  const transport = await import("@/lib/transport");

  const first = transport.Call({ path: "/private/get/user/0.0.0", body: {} });
  await flush();
  const sock = sockets[0];
  sock.onopen();
  sock.onmessage({ data: new Uint8Array([0x03]).buffer });
  await flush();
  answer(sock, sock.sent[sock.sent.length - 1], { user_id: "u1" });
  await first;
  return transport;
}

const watch = (promise) => {
  const state = { settled: false, error: null };
  promise.then(
    () => { state.settled = true; },
    (e) => { state.settled = true; state.error = e; },
  );
  return state;
};

describe("transport request timeouts", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each([
    ["/private/get/user/0.0.0", 30 * 1000],
    ["/private/get/sponsored/buyer/0.0.0", 2 * 60 * 1000],
    ["/private/get/sponsored/quote/0.0.0", 60 * 1000],
    ["/private/post/sponsored/order/0.0.0", 3 * 60 * 1000],
    ["/private/post/import/0.0.0", 10 * 60 * 1000],
  ])("gives %s %i ms before it gives up", async (path, timeoutMs) => {
    const transport = await connectedTransport();
    vi.useFakeTimers();

    const request = watch(transport.Call({ path, body: {} }));
    await vi.advanceTimersByTimeAsync(timeoutMs - 1);
    expect(request.settled).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    expect(request.settled).toBe(true);
    expect(request.error?.code).toBe("ERR_TIMEOUT");
  });

  it("lets a copy check outlive the default budget and still answer", async () => {
    const transport = await connectedTransport();
    vi.useFakeTimers();

    const check = transport.Call({ path: "/private/get/sponsored/buyer/0.0.0", body: { tweet_id: "t1" } });
    const plain = watch(transport.Call({ path: "/private/get/user/0.0.0", body: {} }));
    await vi.advanceTimersByTimeAsync(90 * 1000);
    expect(plain.error?.code).toBe("ERR_TIMEOUT");

    const sock = sockets[0];
    answer(sock, sock.sent[sock.sent.length - 2], { buyer_id: "b1" });
    await expect(check).resolves.toMatchObject({ body: { buyer_id: "b1" } });
  });
});
