// Modified by Warpnet. See NOTICE.

@testable import Env
import XCTest
import SwiftUI
import Network
import WarpnetTransport

@MainActor
final class RouterTests: XCTestCase {
  func testRouterThreadsURL() {
    let router = RouterPath()
    let url = URL(string: "https://www.threads.net/@dimillian")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.isEmpty)
  }
  
  func testRouterTagsURL() {
    let router = RouterPath()
    let url = URL(string: "https://mastodon.social/tags/test")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.isEmpty)
  }
  
  func testRouterLocalStatusURL() {
    let router = RouterPath()
    let client = Client(server: "mastodon.social", session: makeSession())
    client.addConnections(["mastodon.social"])
    router.client = client
    let url = URL(string: "https://mastodon.social/status/1010384")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.first == .statusDetail(id: "1010384"))
  }
  
  func testRouterRemoteStatusURL() {
    let router = RouterPath()
    let client = Client(server: "mastodon.social", session: makeSession())
    client.addConnections(["mastodon.social", "mastodon.online"])
    router.client = client
    let url = URL(string: "https://mastodon.online/status/1010384")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.first == .remoteStatusDetail(url: url))
  }
  
  func testRouteRandomURL() {
    let router = RouterPath()
    let url = URL(string: "https://theweb.com/test/test/one")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.isEmpty)
  }

  private func makeSession() -> WarpnetSession {
    WarpnetSession(binding: StubBinding(),
                   client: WarpnetClient(binding: StubBinding()),
                   store: PairingStore(storage: StubStorage()) { Data(count: $0) },
                   derivePrivateKeyHex: { _, _ in "" })
  }
}

private struct StubBinding: WarpnetBinding {
  func initialize(privKeyHex _: String, warpNetwork _: String, pskHex _: String, bootstrapNodes _: String) -> String { "" }
  func connect(addrInfo _: String) -> String { "" }
  func stream(protocolId _: String, data _: String) -> String { "" }
  func peerId() -> String { "" }
  func isConnected() -> Bool { false }
  func connectedness() -> String { "NotConnected" }
  func disconnect() -> String { "" }
  func pause() {}
  func resume() {}
  func shutdown() -> String { "" }
  func sign(body _: String) -> String { "" }
  func refreshPeerAddrs(_: String) -> String { "" }
  func setLogSink(_: NodeLogSink?) {}
  func setLogSinkMinLevel(_: NodeLogLevel) {}
}

private struct StubStorage: SecretStorage {
  func data(for _: String) -> Data? { nil }
  func set(_: Data, for _: String) throws {}
  func remove(_: String) {}
}
