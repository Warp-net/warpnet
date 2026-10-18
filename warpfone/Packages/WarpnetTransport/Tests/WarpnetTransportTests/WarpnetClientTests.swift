import Foundation
@testable import WarpnetTransport
import XCTest

final class WarpnetClientTests: XCTestCase {
  let config = WarpnetConfig(privKeyHex: "ab", pskHex: "cd", bootstrapAddrs: ["/ip4/1.2.3.4/tcp/1/p2p/B"], network: "testnet")

  func makeClient(_ binding: FakeBinding) -> WarpnetClient {
    WarpnetClient(binding: binding, retryBackoffs: [.milliseconds(1), .milliseconds(1)])
  }

  func connectedClient(_ binding: FakeBinding) async throws -> WarpnetClient {
    let client = makeClient(binding)
    try await client.initialise(config)
    try await client.connect(["/ip4/10.0.0.2/tcp/4001/p2p/N"])
    return client
  }

  func testInitialiseNeedsBootstrapPeers() async {
    let client = makeClient(FakeBinding())
    do {
      try await client.initialise(WarpnetConfig(privKeyHex: "ab", pskHex: "cd", bootstrapAddrs: [], network: "testnet"))
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .transportFailure("bootstrap nodes required"))
    }
    let state = await client.state
    XCTAssertEqual(state, .uninitialised)
  }

  func testInitialisePassesTheConfigToTheBinding() async throws {
    let binding = FakeBinding()
    let client = makeClient(binding)
    try await client.initialise(WarpnetConfig(privKeyHex: "ab", pskHex: "cd", bootstrapAddrs: ["a", "b"], network: "testnet"))
    let state = await client.state
    XCTAssertEqual(state, .disconnected)
    XCTAssertEqual(binding.initializeCalls.first?.bootstrap, "a,b")
    XCTAssertEqual(binding.initializeCalls.first?.network, "testnet")
    try await client.initialise(config)
    XCTAssertEqual(binding.initializeCalls.count, 1)
  }

  func testFailedInitialiseIsReported() async {
    let binding = FakeBinding()
    binding.initializeResult = "invalid PSK: odd length"
    let client = makeClient(binding)
    do {
      try await client.initialise(config)
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .transportFailure("invalid PSK: odd length"))
    }
    let state = await client.state
    XCTAssertEqual(state, .failed(.transportFailure("invalid PSK: odd length")))
  }

  func testConnectDialsEveryCandidateAtOnce() async throws {
    let binding = FakeBinding()
    let client = makeClient(binding)
    try await client.initialise(config)
    try await client.connect(["/a", "/b"])
    XCTAssertEqual(binding.connectCalls, ["/a\n/b"])
    let state = await client.state
    XCTAssertEqual(state, .connected)
  }

  func testConnectFailureMentionsTheFirewall() async throws {
    let binding = FakeBinding()
    binding.connectResults = ["connection failed: dial backoff"]
    let client = makeClient(binding)
    try await client.initialise(config)
    do {
      try await client.connect(["/a"])
      XCTFail("expected a failure")
    } catch let WarpnetError.transportFailure(message) {
      XCTAssertTrue(message.hasPrefix("connection failed: dial backoff."))
      XCTAssertTrue(message.contains("firewall"))
    }
  }

  func testConnectBeforeInitialiseFails() async {
    let client = makeClient(FakeBinding())
    do {
      try await client.connect(["/a"])
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .notInitialised)
    }
  }

  func testRequestSignsAndSendsTheEnvelope() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"{"tweets":[],"cursor":"end"}"#]
    let client = try await connectedClient(binding)
    let body = #"{"user_id":"u1","cursor":"","limit":40}"#
    let raw = try await client.request(ProtocolIds.PRIVATE_GET_TIMELINE, body: body)
    XCTAssertEqual(raw, #"{"tweets":[],"cursor":"end"}"#)

    let call = try XCTUnwrap(binding.streamCalls.first)
    XCTAssertEqual(call.protocolId, ProtocolIds.PRIVATE_GET_TIMELINE)
    let envelope = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(call.data.utf8)) as? [String: Any])
    XCTAssertEqual(envelope["node_id"] as? String, binding.peerIdValue)
    XCTAssertEqual(envelope["path"] as? String, ProtocolIds.PRIVATE_GET_TIMELINE)
    XCTAssertEqual(envelope["signature"] as? String, "c2lnbmF0dXJl")
    XCTAssertEqual(envelope["version"] as? String, "0.0.0")
    XCTAssertEqual((envelope["body"] as? [String: Any])?["user_id"] as? String, "u1")

    let signInput = try XCTUnwrap(binding.signInputs.first)
    XCTAssertTrue(signInput.hasPrefix(body))
    XCTAssertTrue(signInput.hasSuffix(ProtocolIds.PRIVATE_GET_TIMELINE))
    let nanos = signInput.dropFirst(body.count).dropLast(ProtocolIds.PRIVATE_GET_TIMELINE.count)
    XCTAssertEqual(WarpnetTimestamp.rfc3339(nanos: try XCTUnwrap(Int64(nanos))), envelope["timestamp"] as? String)
  }

  func testRequestBeforeConnectFails() async {
    let client = makeClient(FakeBinding())
    do {
      _ = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .notConnected)
    }
  }

  func testAcceptedIsSuccess() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"{"code":0,"message":"Accepted"}"#]
    let client = try await connectedClient(binding)
    let raw = try await client.request(ProtocolIds.PUBLIC_POST_FOLLOW, body: "{}")
    XCTAssertEqual(raw, #"{"code":0,"message":"Accepted"}"#)
  }

  func testUndeliveredRequestsAreRetried() async throws {
    let binding = FakeBinding()
    binding.streamResults = ["failed to open stream: context deadline exceeded", #"{"id":"u"}"#]
    let client = try await connectedClient(binding)
    let raw = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
    XCTAssertEqual(raw, #"{"id":"u"}"#)
    XCTAssertEqual(binding.streamCalls.count, 2)
    let ids = binding.streamCalls.compactMap {
      (try? JSONSerialization.jsonObject(with: Data($0.data.utf8)) as? [String: Any])?["message_id"] as? String
    }
    XCTAssertEqual(Set(ids).count, 2)
  }

  func testRateLimitedRequestsAreRetried() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"{"code":5001,"message":"rate limited"}"#, #"{"id":"u"}"#]
    let client = try await connectedClient(binding)
    _ = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
    XCTAssertEqual(binding.streamCalls.count, 2)
  }

  func testDeliveredRequestsAreNotRetried() async throws {
    let binding = FakeBinding()
    binding.streamResults = ["stream: reading response from P: stream reset", #"{"id":"u"}"#]
    let client = try await connectedClient(binding)
    do {
      _ = try await client.request(ProtocolIds.PRIVATE_POST_TWEET, body: "{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .transportFailure("stream: reading response from P: stream reset"))
    }
    XCTAssertEqual(binding.streamCalls.count, 1)
  }

  func testRetriesStopAtTheBudget() async throws {
    let binding = FakeBinding()
    binding.streamResults = Array(repeating: "failed to open stream: x", count: 5)
    let client = try await connectedClient(binding)
    do {
      _ = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .transportFailure("failed to open stream: x"))
    }
    XCTAssertEqual(binding.streamCalls.count, 3)
  }

  func testServerErrorsAreNotRetried() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"{"code":500,"message":"user not found"}"#]
    let client = try await connectedClient(binding)
    do {
      _ = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .protocolError(code: 500, message: "user not found"))
    }
    XCTAssertEqual(binding.streamCalls.count, 1)
  }

  func testSigningFailureStopsTheRequest() async throws {
    let binding = FakeBinding()
    binding.signResult = "error: private key not set"
    let client = try await connectedClient(binding)
    do {
      _ = try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .signingUnavailable("error: private key not set"))
    }
    XCTAssertTrue(binding.streamCalls.isEmpty)
  }

  func testPairRefreshesThePeerAddresses() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"["/ip4/1.2.3.4/tcp/4001","/ip4/10.0.0.2/tcp/4001"]"#]
    let client = try await connectedClient(binding)
    try await client.pair(#"{"token":"t"}"#)
    XCTAssertEqual(binding.streamCalls.first?.protocolId, ProtocolIds.PRIVATE_POST_PAIR)
    XCTAssertTrue(binding.streamCalls.first?.data.hasPrefix(#"{"body":{"token":"t"},"#) ?? false)
    XCTAssertEqual(binding.refreshCalls, ["/ip4/1.2.3.4/tcp/4001\n/ip4/10.0.0.2/tcp/4001"])
  }

  func testPairRejectionIsAProtocolError() async throws {
    let binding = FakeBinding()
    binding.streamResults = [#"{"code":500,"message":"token mismatch"}"#]
    let client = try await connectedClient(binding)
    do {
      try await client.pair("{}")
      XCTFail("expected a failure")
    } catch let error as WarpnetError {
      XCTAssertEqual(error, .protocolError(code: 500, message: "token mismatch"))
      XCTAssertTrue(error.isDurablePairRejection)
    }
  }

  func testPairTransportFailureIsReported() async throws {
    let binding = FakeBinding()
    binding.streamResults = ["failed to open stream: peer id mismatch"]
    let client = try await connectedClient(binding)
    do {
      try await client.pair("{}")
      XCTFail("expected a failure")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .transportFailure("unexpected pairing response: failed to open stream: peer id mismatch"))
    }
  }

  func testPausedClientFailsFastAndResumeDoesNotWaitForTheRedial() async throws {
    let binding = FakeBinding()
    let client = try await connectedClient(binding)
    await client.pause()
    var state = await client.state
    XCTAssertEqual(state, .disconnected)
    XCTAssertEqual(binding.pauseCount, 1)
    do {
      _ = try await client.request("/p", body: "{}")
      XCTFail("a paused client must not dial")
    } catch {
      XCTAssertEqual(error as? WarpnetError, .notConnected)
    }

    binding.isConnectedResult = false
    await client.resume()
    state = await client.state
    XCTAssertEqual(state, .connected)
    XCTAssertEqual(binding.resumeCount, 1)
    _ = try await client.request("/p", body: "{}")
    XCTAssertEqual(binding.streamCalls.count, 1)
  }

  func testResumeOnlyRestoresAConnectionThePauseTookDown() async throws {
    let binding = FakeBinding()
    let client = try await connectedClient(binding)
    await client.pause()
    await client.shutdown()
    try await client.initialise(config)
    await client.resume()
    let state = await client.state
    XCTAssertEqual(state, .disconnected)
  }

  func testRequestWaitsForAReconnectInFlight() async throws {
    let binding = FakeBinding()
    let client = try await connectedClient(binding)
    let gate = DispatchSemaphore(value: 0)
    binding.connectGate = gate
    let reconnect = Task { try await client.connect(["/a"]) }
    while await client.state != .connecting {
      await Task.yield()
    }
    let request = Task { try await client.request("/p", body: "{}") }
    try await Task.sleep(for: .milliseconds(50))
    XCTAssertTrue(binding.streamCalls.isEmpty)
    gate.signal()
    try await reconnect.value
    _ = try await request.value
    XCTAssertEqual(binding.streamCalls.count, 1)
  }

  func testResumeKeepsAFailure() async throws {
    let binding = FakeBinding()
    binding.connectResults = ["connection failed"]
    let client = makeClient(binding)
    try await client.initialise(config)
    try? await client.connect(["/a"])
    await client.resume()
    let state = await client.state
    guard case .failed = state else { return XCTFail("expected the failure to stay, got \(state)") }
  }

  func testShutdownReturnsToUninitialised() async throws {
    let binding = FakeBinding()
    let client = try await connectedClient(binding)
    await client.shutdown()
    let state = await client.state
    XCTAssertEqual(state, .uninitialised)
    XCTAssertEqual(binding.shutdownCount, 1)
  }

  func testConcurrentRequestsDoNotSerialise() async throws {
    let binding = FakeBinding()
    let client = try await connectedClient(binding)
    try await withThrowingTaskGroup(of: String.self) { group in
      for _ in 0 ..< 20 {
        group.addTask { try await client.request(ProtocolIds.PUBLIC_GET_USER, body: "{}") }
      }
      for try await raw in group {
        XCTAssertEqual(raw, "{}")
      }
    }
    XCTAssertEqual(binding.streamCalls.count, 20)
  }
}
