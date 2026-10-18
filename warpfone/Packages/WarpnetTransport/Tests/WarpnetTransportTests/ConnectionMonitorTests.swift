import Foundation
@testable import WarpnetTransport
import XCTest

final class ConnectionMonitorTests: XCTestCase {
  let config = WarpnetConfig(privKeyHex: "ab", pskHex: "cd", bootstrapAddrs: ["/b"], network: "testnet")

  @MainActor
  func makeMonitor(_ binding: FakeBinding,
                   _ client: WarpnetClient,
                   dialAddresses: [String] = ["/ip4/10.0.0.2/tcp/4001/p2p/N"]) -> ConnectionMonitor
  {
    ConnectionMonitor(client: client,
                      binding: binding,
                      pollInterval: .milliseconds(5),
                      reconnectBackoffs: [.milliseconds(1)],
                      dialAddresses: { dialAddresses })
  }

  @MainActor
  func waitFor(_ monitor: ConnectionMonitor,
               _ condition: @escaping (LinkState) -> Bool,
               file: StaticString = #filePath,
               line: UInt = #line) async
  {
    for _ in 0 ..< 400 {
      if condition(monitor.linkState) { return }
      try? await Task.sleep(for: .milliseconds(5))
    }
    XCTFail("link state stayed \(monitor.linkState)", file: file, line: line)
  }

  @MainActor
  func testUninitialisedClientIsUnknown() async {
    let binding = FakeBinding()
    let monitor = makeMonitor(binding, WarpnetClient(binding: binding))
    await monitor.poll()
    XCTAssertEqual(monitor.linkState, .unknown)
    XCTAssertTrue(binding.connectCalls.isEmpty)
  }

  @MainActor
  func testLimitedLinkIsPublished() async throws {
    let binding = FakeBinding()
    binding.connectednessResults = ["Limited"]
    let client = WarpnetClient(binding: binding)
    try await client.initialise(config)
    let monitor = makeMonitor(binding, client)
    await monitor.poll()
    XCTAssertEqual(monitor.linkState, .limited)
  }

  @MainActor
  func testDroppedLinkIsRedialled() async throws {
    let binding = FakeBinding()
    let client = WarpnetClient(binding: binding)
    try await client.initialise(config)
    binding.linkUp = false
    let monitor = makeMonitor(binding, client)
    monitor.start()
    await waitFor(monitor) { $0 == .connected }
    monitor.stop()
    XCTAssertEqual(binding.connectCalls, ["/ip4/10.0.0.2/tcp/4001/p2p/N"])
    let state = await client.state
    XCTAssertEqual(state, .connected)
  }

  @MainActor
  func testFailedDialsBackOffAndRetry() async throws {
    let binding = FakeBinding()
    binding.connectResults = ["connection failed", "connection failed"]
    let client = WarpnetClient(binding: binding)
    try await client.initialise(config)
    binding.linkUp = false
    let monitor = makeMonitor(binding, client)
    monitor.start()
    await waitFor(monitor) { $0 == .connected }
    monitor.stop()
    XCTAssertEqual(binding.connectCalls.count, 3)
  }

  @MainActor
  func testNothingToDialLeavesTheLinkDown() async throws {
    let binding = FakeBinding()
    let client = WarpnetClient(binding: binding)
    try await client.initialise(config)
    binding.linkUp = false
    let monitor = makeMonitor(binding, client, dialAddresses: [])
    await monitor.poll()
    await waitFor(monitor) { $0 == .notConnected }
    XCTAssertTrue(binding.connectCalls.isEmpty)
  }

  @MainActor
  func testStopResetsTheState() async throws {
    let binding = FakeBinding()
    let client = WarpnetClient(binding: binding)
    try await client.initialise(config)
    let monitor = makeMonitor(binding, client)
    await monitor.poll()
    XCTAssertEqual(monitor.linkState, .connected)
    monitor.stop()
    XCTAssertEqual(monitor.linkState, .unknown)
  }
}
