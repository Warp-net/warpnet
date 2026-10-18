import Foundation
@testable import WarpnetTransport
import XCTest

final class WarpnetEnvelopeTests: XCTestCase {
  func testTimestampIsRFC3339WithNanoseconds() {
    XCTAssertEqual(WarpnetTimestamp.rfc3339(nanos: 0), "1970-01-01T00:00:00.000000000Z")
    XCTAssertEqual(WarpnetTimestamp.rfc3339(nanos: 1_700_000_000_123_456_000), "2023-11-14T22:13:20.123456000Z")
    XCTAssertEqual(WarpnetTimestamp.rfc3339(nanos: 951_782_400_000_000_001), "2000-02-29T00:00:00.000000001Z")
    XCTAssertEqual(WarpnetTimestamp.rfc3339(nanos: 4_102_444_799_999_999_999), "2099-12-31T23:59:59.999999999Z")
  }

  func testNowIsMicrosecondAligned() {
    let nanos = WarpnetTimestamp.nowNanos()
    XCTAssertEqual(nanos % 1000, 0)
    XCTAssertGreaterThan(nanos, 1_700_000_000_000_000_000)
  }

  func testJsonCarriesTheBodyVerbatim() throws {
    let envelope = WarpnetEnvelope(body: #"{"user_id":"u1","limit":40}"#,
                                   nodeId: "12D3KooWPeer",
                                   path: ProtocolIds.PRIVATE_GET_TIMELINE,
                                   timestampNanos: 1_700_000_000_123_456_000,
                                   messageId: "m-1",
                                   signature: "c2ln")
    XCTAssertEqual(
      envelope.json(),
      #"{"body":{"user_id":"u1","limit":40},"message_id":"m-1","node_id":"12D3KooWPeer","path":"/private/get/timeline/0.0.0","timestamp":"2023-11-14T22:13:20.123456000Z","version":"0.0.0","signature":"c2ln"}"#
    )
    let decoded = try JSONSerialization.jsonObject(with: Data(envelope.json().utf8)) as? [String: Any]
    XCTAssertEqual((decoded?["body"] as? [String: Any])?["user_id"] as? String, "u1")
  }

  func testSigningInputIsBodyThenNanosThenPath() {
    let envelope = WarpnetEnvelope(body: "{}", nodeId: "n", path: "/p/0.0.0", timestampNanos: 42)
    XCTAssertEqual(envelope.signingInput, "{}42/p/0.0.0")
  }

  func testMessageIdsAreUnique() {
    let first = WarpnetEnvelope(body: "{}", nodeId: "n", path: "/p", timestampNanos: 1)
    let second = WarpnetEnvelope(body: "{}", nodeId: "n", path: "/p", timestampNanos: 1)
    XCTAssertFalse(first.messageId.isEmpty)
    XCTAssertNotEqual(first.messageId, second.messageId)
  }

  func testStringFieldsAreEscaped() {
    XCTAssertEqual(jsonQuoted("a\"b\\c\nd\u{01}é"), #""a\"b\\c\nd\u0001é""#)
  }

  func testResponseClassification() {
    XCTAssertNoThrow(try WarpnetClient.throwIfErrorResponse(""))
    XCTAssertNoThrow(try WarpnetClient.throwIfErrorResponse(#"{"code":0,"message":"Accepted"}"#))
    XCTAssertNoThrow(try WarpnetClient.throwIfErrorResponse(#"{"code":0,"message":"empty response"}"#))
    XCTAssertNoThrow(try WarpnetClient.throwIfErrorResponse(#"{"id":"t1","text":"hi"}"#))
    XCTAssertNoThrow(try WarpnetClient.throwIfErrorResponse(#"["/ip4/1.2.3.4/tcp/1"]"#))
    XCTAssertThrowsError(try WarpnetClient.throwIfErrorResponse(#"{"code":500,"message":"boom"}"#)) {
      XCTAssertEqual($0 as? WarpnetError, .protocolError(code: 500, message: "boom"))
    }
    XCTAssertThrowsError(try WarpnetClient.throwIfErrorResponse("failed to open stream: context deadline exceeded")) {
      XCTAssertEqual($0 as? WarpnetError, .transportFailure("failed to open stream: context deadline exceeded"))
    }
  }

  func testRetryPolicy() {
    XCTAssertTrue(WarpnetClient.isRetryable(.transportFailure("failed to open stream: protocols not supported")))
    XCTAssertTrue(WarpnetClient.isRetryable(.transportFailure("not connected to desktop node")))
    XCTAssertTrue(WarpnetClient.isRetryable(.transportFailure("context deadline exceeded")))
    XCTAssertTrue(WarpnetClient.isRetryable(.transportFailure("stream reset")))
    XCTAssertTrue(WarpnetClient.isRetryable(.protocolError(code: 5001, message: "rate limited")))
    XCTAssertFalse(WarpnetClient.isRetryable(.transportFailure("stream: reading response from P: stream reset")))
    XCTAssertFalse(WarpnetClient.isRetryable(.protocolError(code: 500, message: "boom")))
    XCTAssertFalse(WarpnetClient.isRetryable(.signingUnavailable("error: x")))
    XCTAssertFalse(WarpnetClient.isRetryable(.notConnected))
  }

  func testDurablePairRejections() {
    XCTAssertTrue(WarpnetError.protocolError(code: 500, message: "token mismatch").isDurablePairRejection)
    XCTAssertTrue(WarpnetError.protocolError(code: 500, message: "Pairing revoked").isDurablePairRejection)
    XCTAssertTrue(WarpnetError.protocolError(code: 500, message: "too many aliases").isDurablePairRejection)
    XCTAssertFalse(WarpnetError.protocolError(code: 500, message: "database closed").isDurablePairRejection)
    XCTAssertFalse(WarpnetError.transportFailure("token mismatch").isDurablePairRejection)
  }

  func testLinkStateMapsBindingValues() {
    XCTAssertEqual(LinkState(binding: "Connected"), .connected)
    XCTAssertEqual(LinkState(binding: "Limited"), .limited)
    XCTAssertEqual(LinkState(binding: "NotConnected"), .notConnected)
    XCTAssertEqual(LinkState(binding: "CanConnect"), .notConnected)
    XCTAssertEqual(LinkState(binding: "CannotConnect"), .notConnected)
    XCTAssertEqual(LinkState(binding: "Uninitialised"), .unknown)
    XCTAssertTrue(LinkState.limited.isUp)
    XCTAssertFalse(LinkState.reconnecting(attempt: 1).isUp)
  }
}
