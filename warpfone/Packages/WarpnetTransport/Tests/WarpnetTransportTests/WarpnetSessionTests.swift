import Foundation
@testable import WarpnetTransport
import XCTest

final class WarpnetSessionTests: XCTestCase {
  static let info = AuthNodeInfo(token: "t",
                                 psk: "ab",
                                 network: "testnet",
                                 addresses: ["/ip4/10.0.0.2/tcp/4001", "/ip4/127.0.0.1/tcp/4001"],
                                 nodeId: "NODE",
                                 userId: "USER",
                                 bootstrapPeers: [])
  static let rawJson = #"{"token":"t","psk":"ab","network":"testnet","addresses":["/ip4/10.0.0.2/tcp/4001","/ip4/127.0.0.1/tcp/4001"],"node_id":"NODE","user_id":"USER"}"#

  func makeSession(_ binding: FakeBinding,
                   storage: MemoryStorage = MemoryStorage(),
                   repairInterval: TimeInterval = 3600) -> WarpnetSession
  {
    let store = PairingStore(storage: storage) { Data(repeating: 7, count: $0) }
    return WarpnetSession(binding: binding,
                          client: WarpnetClient(binding: binding, retryBackoffs: []),
                          store: store,
                          repairInterval: repairInterval) { peer, seeds in
      "key-\(peer)-\(try seeds.seed(memberPeerId: peer).count)"
    }
  }

  func testPairingRunsTheHandshakeAndPersistsIt() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"["/ip4/10.0.0.2/tcp/4001"]"#]
    let storage = MemoryStorage()
    let session = makeSession(binding, storage: storage)

    let outcome = await session.pair(Self.info, rawJson: Self.rawJson)
    XCTAssertEqual(outcome, .success(Self.info))
    XCTAssertEqual(binding.initializeCalls.first?.privKeyHex, "key-NODE-32")
    XCTAssertEqual(binding.initializeCalls.first?.bootstrap, "/ip4/10.0.0.2/tcp/4001/p2p/NODE")
    XCTAssertEqual(binding.connectCalls, ["/ip4/10.0.0.2/tcp/4001/p2p/NODE"])
    XCTAssertEqual(binding.streamCalls.first?.protocolId, ProtocolIds.PRIVATE_POST_PAIR)
    XCTAssertEqual(session.store.loadRawQr(), Self.rawJson)
    XCTAssertEqual(storage.data(for: PairingStore.identitySeedKey), Data(repeating: 7, count: 32))
    let pairing = await session.pairing
    XCTAssertEqual(pairing, Self.info)
    XCTAssertEqual(session.storedPairing, Self.info)
  }

  func testRejectionIsReportedAndNothingIsSaved() async {
    let binding = FakeBinding()
    binding.streamResults = [#"{"code":500,"message":"token mismatch"}"#]
    let session = makeSession(binding)
    let outcome = await session.pair(Self.info, rawJson: Self.rawJson)
    XCTAssertEqual(outcome, .rejected(code: 500, message: "token mismatch"))
    XCTAssertNil(session.store.loadRawQr())
  }

  func testUnreachableNodeIsATransportError() async {
    let binding = FakeBinding()
    binding.connectResults = ["connection failed: dial backoff"]
    let session = makeSession(binding)
    guard case let .transportError(message) = await session.pair(Self.info, rawJson: Self.rawJson) else {
      return XCTFail("expected a transport error")
    }
    XCTAssertTrue(message.contains("dial backoff"))
  }

  func testPeerIdMismatchIsReported() async {
    let binding = FakeBinding()
    binding.connectResults = ["connection failed: failed to negotiate security protocol: peer id mismatch"]
    let session = makeSession(binding)
    guard case let .peerIdMismatch(expected, _) = await session.pair(Self.info, rawJson: Self.rawJson) else {
      return XCTFail("expected a peer id mismatch")
    }
    XCTAssertEqual(expected, "NODE")
  }

  func testRepairingShutsTheOldHostDown() async {
    let binding = FakeBinding()
    binding.streamResults = ["[]", "[]"]
    let session = makeSession(binding)
    _ = await session.pair(Self.info, rawJson: Self.rawJson)
    _ = await session.pair(Self.info, rawJson: Self.rawJson)
    XCTAssertEqual(binding.shutdownCount, 1)
    XCTAssertEqual(binding.initializeCalls.count, 2)
  }

  func testRestoreRepairsFromTheStoredPayload() async throws {
    let binding = FakeBinding()
    binding.streamResults = ["[]", "[]"]
    let storage = MemoryStorage()
    _ = await makeSession(binding, storage: storage).pair(Self.info, rawJson: Self.rawJson)

    let restored = makeSession(binding, storage: storage)
    let outcome = await restored.restore()
    XCTAssertEqual(outcome, .success(Self.info))
    XCTAssertEqual(binding.streamCalls.last?.protocolId, ProtocolIds.PRIVATE_POST_PAIR)
  }

  func testRestoreWithoutAPairingDoesNothing() async {
    let binding = FakeBinding()
    let outcome = await makeSession(binding).restore()
    XCTAssertNil(outcome)
    XCTAssertTrue(binding.initializeCalls.isEmpty)
  }

  func testRevokedPairingIsForgottenOnRestore() async throws {
    let binding = FakeBinding()
    let storage = MemoryStorage()
    try storage.set(Data(Self.rawJson.utf8), for: PairingStore.rawQrKey)
    binding.streamResults = [#"{"code":500,"message":"pairing revoked"}"#]
    let session = makeSession(binding, storage: storage)
    let outcome = await session.restore()
    XCTAssertEqual(outcome, .rejected(code: 500, message: "pairing revoked"))
    XCTAssertNil(session.store.loadRawQr())
  }

  func testTransportErrorKeepsThePairingForTheNextLaunch() async throws {
    let binding = FakeBinding()
    let storage = MemoryStorage()
    try storage.set(Data(Self.rawJson.utf8), for: PairingStore.rawQrKey)
    binding.connectResults = ["connection failed"]
    let session = makeSession(binding, storage: storage)
    guard case .transportError = await session.restore() else { return XCTFail("expected a transport error") }
    XCTAssertEqual(session.store.loadRawQr(), Self.rawJson)
  }

  func testResumeRepairsOnlyAfterTheInterval() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"["/ip4/10.0.0.2/tcp/4001"]"#, #"["/ip4/10.0.0.2/tcp/4001"]"#]
    let fresh = makeSession(binding)
    _ = await fresh.pair(Self.info, rawJson: Self.rawJson)
    let skipped = await fresh.resume()
    XCTAssertNil(skipped)
    XCTAssertEqual(binding.streamCalls.count, 1)

    let stale = makeSession(binding, repairInterval: 0)
    _ = await stale.pair(Self.info, rawJson: Self.rawJson)
    binding.streamResults = [#"["/ip4/10.0.0.2/tcp/4001"]"#]
    let refreshed = await stale.resume()
    XCTAssertEqual(refreshed, .success(Self.info))
    XCTAssertEqual(binding.streamCalls.last?.protocolId, ProtocolIds.PRIVATE_POST_PAIR)
  }

  func testResumeForgetsARevokedPairing() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"["/ip4/10.0.0.2/tcp/4001"]"#]
    let session = makeSession(binding, repairInterval: 0)
    _ = await session.pair(Self.info, rawJson: Self.rawJson)
    binding.streamResults = [#"{"code":5000,"message":"pairing revoked"}"#]
    let outcome = await session.resume()
    XCTAssertEqual(outcome, .rejected(code: 5000, message: "pairing revoked"))
    XCTAssertNil(session.store.loadRawQr())
    let pairing = await session.pairing
    XCTAssertNil(pairing)
  }

  func testUnpairRemovesThisDeviceAndForgetsThePairing() async throws {
    let binding = FakeBinding()
    binding.streamResults = ["[]", #"{"devices":[]}"#]
    let session = makeSession(binding)
    _ = await session.pair(Self.info, rawJson: Self.rawJson)
    await session.unpair()

    let call = try XCTUnwrap(binding.streamCalls.last)
    XCTAssertEqual(call.protocolId, ProtocolIds.PRIVATE_DELETE_DEVICE)
    XCTAssertTrue(call.data.contains(#""body":{"node_id":"\#(binding.peerIdValue)"}"#))
    XCTAssertNil(session.store.loadRawQr())
    let pairing = await session.pairing
    XCTAssertNil(pairing)
    let state = await session.client.state
    XCTAssertEqual(state, .uninitialised)
  }

  func testDialCandidatesFollowThePairing() async {
    let binding = FakeBinding()
    binding.streamResults = ["[]"]
    let session = makeSession(binding)
    let before = await session.dialCandidates()
    XCTAssertTrue(before.isEmpty)
    _ = await session.pair(Self.info, rawJson: Self.rawJson)
    let after = await session.dialCandidates()
    XCTAssertEqual(after, ["/ip4/10.0.0.2/tcp/4001/p2p/NODE"])
  }
}
