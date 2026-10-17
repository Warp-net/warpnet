// Modified by Warpnet. See NOTICE.

@testable import Timeline
import XCTest
import Network
import Models


final class TimelineFilterTests: XCTestCase {
  func testCodableHome() throws {
    XCTAssertTrue(try testCodableOn(filter: .home))
  }
  
  private func testCodableOn(filter: TimelineFilter) throws -> Bool {
    let encoder = JSONEncoder()
    let decoder = JSONDecoder()
    let data = try encoder.encode(filter)
    let newFilter = try decoder.decode(TimelineFilter.self, from: data)
    return newFilter == filter
  }
}
