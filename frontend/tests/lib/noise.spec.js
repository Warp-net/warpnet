import { describe, it, expect } from "vitest";
import { NoiseInitiator, fingerprint } from "@/lib/noise";

function fromHex(hex) {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return out;
}

function toHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

// Official Noise_NX_25519_ChaChaPoly_SHA256 test vector
// (vendor/github.com/flynn/noise/vectors.txt).
const VECTOR = {
  initStaticPriv: "2020202020202020202020202020202020202020202020202020202020202020",
  initEphemeralPriv: "4040404040404040404040404040404040404040404040404040404040404040",
  respStaticPub: "e50c239bc204f1341664c9d9c50c6a0d0fff6fc79d9301f1e713aab2e0344b3f",
  msg1: "d7b5e81d336e578b13b8d706e82d061e3038c96bce66cdcf50d566b96ddbba10",
  msg2: "ef38b4abd14b0a919cbe6839f6185b97d32607bee359c6c53c12cc7597867d59b91c9dc0a0820717ab98d7884c0374fd5a08eb1d378599ad79e652304dac93adfe8a8e360f0b15fde9306776ed327335aad2999c1ff889bffb258c59f6230764",
  msg3: "3947175440092392aac66836380f0d934414af732dc68f5129eafeac02fd4671216c51259122c5a18122302b876f2c9df184223a8a5b71f31fa2dce281e6086a",
  payload: "79656c6c6f777375626d6172696e65",
  fromInitiator: "a177e6308e3442bb59761cd6ef55c59d800a81baecf010c71f10ec44b1da38",
  fromResponder: "25dbe368f7b90458058631ccfc6342928fcc1b8510910c040475fd2b0c1cb7",
};

const newInitiator = () =>
  new NoiseInitiator(fromHex(VECTOR.initStaticPriv), fromHex(VECTOR.initEphemeralPriv));

function handshake() {
  const initiator = newInitiator();
  initiator.writeMessageA();
  initiator.readMessageB(fromHex(VECTOR.msg2));
  return initiator.writeMessageC();
}

describe("Noise XX initiator", () => {
  it("reproduces the Go peer's handshake end to end", () => {
    const initiator = newInitiator();

    expect(toHex(initiator.writeMessageA())).toBe(VECTOR.msg1);

    initiator.readMessageB(fromHex(VECTOR.msg2));
    const { message, session } = initiator.writeMessageC();

    expect(toHex(message)).toBe(VECTOR.msg3, "the node must recognise our static key");
    expect(toHex(session.remoteStatic)).toBe(VECTOR.respStaticPub);
    expect(toHex(session.encrypt(fromHex(VECTOR.payload)))).toBe(VECTOR.fromInitiator);
    expect(toHex(session.decrypt(fromHex(VECTOR.fromResponder)))).toBe(VECTOR.payload);
  });

  it("advances counter nonces so identical plaintexts differ", () => {
    const { session } = handshake();

    const a = session.encrypt(new TextEncoder().encode("same"));
    const b = session.encrypt(new TextEncoder().encode("same"));
    expect(toHex(a)).not.toBe(toHex(b));
  });

  it("rejects a tampered handshake response", () => {
    const initiator = newInitiator();
    initiator.writeMessageA();
    const evil = fromHex(VECTOR.msg2);
    evil[evil.length - 1] ^= 0xff;
    expect(() => initiator.readMessageB(evil)).toThrow();
  });

  it("rejects a tampered transport frame", () => {
    const { session } = handshake();
    const evil = fromHex(VECTOR.fromResponder);
    evil[0] ^= 0xff;
    expect(() => session.decrypt(evil)).toThrow();
  });

  it("computes the same fingerprint format as the node", () => {
    const fp = fingerprint(fromHex(VECTOR.respStaticPub));
    expect(fp).toMatch(/^[0-9a-f]{64}$/);
  });
});
