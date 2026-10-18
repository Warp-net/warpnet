// Modified by Warpnet. See NOTICE.

@testable import Env
import XCTest
import SwiftUI

@MainActor
final class RouterTests: XCTestCase {
  func testRouterSkipsWwwProfileURL() {
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

  func testRouteRandomURL() {
    let router = RouterPath()
    let url = URL(string: "https://theweb.com/test/test/one")!
    _ = router.handle(url: url)
    XCTAssertTrue(router.path.isEmpty)
  }
}
