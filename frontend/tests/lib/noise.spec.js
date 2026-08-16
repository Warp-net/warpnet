import { describe, it, expect } from "vitest";
import { NoiseInitiator, fingerprint } from "@/lib/noise";
import { x25519 } from "@noble/curves/ed25519";

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

// Official Noise_NX_25519_ChaChaPoly_SHA256 test vector — the same one the
// node's responder library ships (vendor/github.com/flynn/noise/vectors.txt).
// Passing it proves wire-level compatibility with the Go side.
const VECTOR = {
  respStaticPriv: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
  initEphemeralPriv: "202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f",
  msg0: "358072d6365880d1aeea329adf9121383851ed21a28e3b75e965d0d2cd166254",
  msg1:
    "64b101b1d0be5a8704bd078f9895001fc03e8e9f9522f188dd128d9846d4846686b5f4e8c51a605bcb276206a6df60ae938b905adaf29a2dae4a4951bbd9ac64830ab64f2329646560b930979ff52da8dda7c0677c502dba13c078b5afd1bf11",
  msg2Payload: "79656c6c6f777375626d6172696e65", // "yellowsubmarine"
  msg2: "92613cda6ccb2936449efb8ff870b5a4536f5734a4e31056d38101230762e8",
  msg3Payload: "7375626d6172696e6579656c6c6f77", // "submarineyellow"
  msg3: "ed89355072429afe6c3442ba7af66f6647499291bab58d40f6a392e79ff80a",
};

describe("Noise NX initiator", () => {
  it("reproduces the official test vector end to end", () => {
    const initiator = new NoiseInitiator(fromHex(VECTOR.initEphemeralPriv));

    expect(toHex(initiator.writeMessageA())).toBe(VECTOR.msg0);

    const session = initiator.readMessageB(fromHex(VECTOR.msg1));

    // TOFU material: the learned static key is exactly the responder's.
    const respStaticPub = x25519.getPublicKey(fromHex(VECTOR.respStaticPriv));
    expect(toHex(session.remoteStatic)).toBe(toHex(respStaticPub));

    // First transport frame initiator → responder.
    const sealed = session.encrypt(fromHex(VECTOR.msg2Payload));
    expect(toHex(sealed)).toBe(VECTOR.msg2);

    // First transport frame responder → initiator.
    const plain = session.decrypt(fromHex(VECTOR.msg3));
    expect(toHex(plain)).toBe(VECTOR.msg3Payload);
  });

  it("advances counter nonces so identical plaintexts differ", () => {
    const initiator = new NoiseInitiator(fromHex(VECTOR.initEphemeralPriv));
    initiator.writeMessageA();
    const session = initiator.readMessageB(fromHex(VECTOR.msg1));

    const a = session.encrypt(new TextEncoder().encode("same"));
    const b = session.encrypt(new TextEncoder().encode("same"));
    expect(toHex(a)).not.toBe(toHex(b));
  });

  it("rejects a tampered handshake response", () => {
    const initiator = new NoiseInitiator(fromHex(VECTOR.initEphemeralPriv));
    initiator.writeMessageA();
    const evil = fromHex(VECTOR.msg1);
    evil[evil.length - 1] ^= 0xff;
    expect(() => initiator.readMessageB(evil)).toThrow();
  });

  it("rejects a tampered transport frame", () => {
    const initiator = new NoiseInitiator(fromHex(VECTOR.initEphemeralPriv));
    initiator.writeMessageA();
    const session = initiator.readMessageB(fromHex(VECTOR.msg1));
    const evil = fromHex(VECTOR.msg3);
    evil[0] ^= 0xff;
    expect(() => session.decrypt(evil)).toThrow();
  });

  it("computes the same fingerprint format as the node", () => {
    const fp = fingerprint(x25519.getPublicKey(fromHex(VECTOR.respStaticPriv)));
    expect(fp).toMatch(/^[0-9a-f]{64}$/);
  });
});
