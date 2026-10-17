import Foundation
@testable import WarpnetTransport
import XCTest

final class QrPayloadTests: XCTestCase {
  static let pairingJson = #"{"user_id":"01JAUSER","token":"secret-token","psk":"0000000000000000000000000000000000000000000000000000000000000000","node_id":"12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j","addresses":["/ip4/192.168.1.10/tcp/4001","/ip4/127.0.0.1/tcp/4001"],"bootstrap_peers":["/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j"],"network":"testnet"}"#

  static let pairingPayload = "K:3N01000000KB0K+KQVPIUOS62H5JV:0:VTEVV4EGD-0J67.G9+PL9L9HU2YA20LM6C8/:OEWS0LA%93I3APN6MC3E809JIIKDI6C9H1C9MUBQS2A%90X0JMG7CPRZSS.$3NGSKTF%1W NIQ%E0TVE*Q4$NV.DO-IC6F.ZSXZKFN3B14T:EE VJ%F:0FXNVD2MHYU:$EI4M9SHH7VHCVW5A:I2D-87563DGVSVDRSYL16D3NR2CR4MZNC IA%KTB9/12NY5THJN 0HATCMEZ3W2L2G2CPI8$SBW05..UBVP8V23B4GKVU-G0CSDE4WR0P08CR6$/PV5000"

  func testBase45MatchesRFC9285Vectors() throws {
    XCTAssertEqual(Base45.encode(Data("AB".utf8)), "BB8")
    XCTAssertEqual(Base45.encode(Data("Hello!!".utf8)), "%69 VD92EX0")
    XCTAssertEqual(Base45.encode(Data("base-45".utf8)), "UJCLQE7W581")
    XCTAssertEqual(try Base45.decode("QED8WEX0"), Data("ietf!".utf8))
    XCTAssertEqual(try Base45.decode(""), Data())
  }

  func testBase45RejectsMalformedInput() {
    XCTAssertThrowsError(try Base45.decode("A")) {
      XCTAssertEqual($0 as? Base45.DecodingError, .invalidLength)
    }
    XCTAssertThrowsError(try Base45.decode("abc")) {
      XCTAssertEqual($0 as? Base45.DecodingError, .invalidCharacter)
    }
    XCTAssertThrowsError(try Base45.decode(":::")) {
      XCTAssertEqual($0 as? Base45.DecodingError, .valueOutOfRange)
    }
    XCTAssertThrowsError(try Base45.decode("ÄÄÄ")) {
      XCTAssertEqual($0 as? Base45.DecodingError, .invalidCharacter)
    }
  }

  func testBase45RoundTripsEveryByte() throws {
    let bytes = Data((0 ... 255).map { UInt8($0) })
    XCTAssertEqual(try Base45.decode(Base45.encode(bytes)), bytes)
    XCTAssertEqual(try Base45.decode(Base45.encode(bytes.dropLast())), bytes.dropLast())
  }

  func testPlainJsonPassesThrough() throws {
    XCTAssertEqual(try QrPayloadCodec.decode("  {\"a\":1}\n"), "{\"a\":1}")
    XCTAssertEqual(try QrPayloadCodec.decode("   "), "")
  }

  func testCRC32MatchesTheCheckValue() {
    XCTAssertEqual(QrPayloadCodec.crc32(Data("123456789".utf8)), 0xCBF4_3926)
  }

  func testNonGzipPayloadIsRejected() {
    XCTAssertThrowsError(try QrPayloadCodec.decode(Base45.encode(Data("not gzip at all, really".utf8)))) {
      XCTAssertEqual($0 as? QrPayloadCodec.DecodingError, .notGzip)
    }
  }

  func testOversizedPayloadIsRejectedBeforeDecoding() {
    let payload = String(repeating: "A", count: QrPayloadCodec.maxEncodedChars + 3)
    XCTAssertThrowsError(try QrPayloadCodec.decode(payload)) {
      XCTAssertEqual($0 as? QrPayloadCodec.DecodingError, .tooLong)
    }
  }

  func testDeclaredSizeOverTheLimitIsRejected() throws {
    var gzip = try Base45.decode(Self.pairingPayload)
    let count = gzip.count
    gzip.replaceSubrange(count - 4 ..< count, with: [0x00, 0x00, 0x10, 0x00])
    XCTAssertThrowsError(try QrPayloadCodec.gunzip(gzip, limit: QrPayloadCodec.maxDecodedBytes)) {
      XCTAssertEqual($0 as? QrPayloadCodec.DecodingError, .tooLarge)
    }
  }

  #if canImport(Darwin)
    func testFrontendPayloadDecodes() throws {
      XCTAssertEqual(try QrPayloadCodec.decode(Self.pairingPayload), Self.pairingJson)
    }

    func testCorruptedPayloadFailsTheChecksum() throws {
      var gzip = try Base45.decode(Self.pairingPayload)
      let crcIndex = gzip.count - 8
      gzip[crcIndex] ^= 0xFF
      XCTAssertThrowsError(try QrPayloadCodec.gunzip(gzip, limit: QrPayloadCodec.maxDecodedBytes)) {
        XCTAssertEqual($0 as? QrPayloadCodec.DecodingError, .checksumMismatch)
      }
    }
  #endif

  func testValidatorAcceptsAPlainJsonPairing() {
    guard case let .valid(info, rawJson) = AuthNodeInfoValidator.validate(Self.pairingJson) else {
      return XCTFail("expected a valid pairing")
    }
    XCTAssertEqual(rawJson, Self.pairingJson)
    XCTAssertEqual(info.token, "secret-token")
    XCTAssertEqual(info.network, "testnet")
    XCTAssertEqual(info.userId, "01JAUSER")
    XCTAssertEqual(info.dialCandidates, [
      "/ip4/192.168.1.10/tcp/4001/p2p/12D3KooWMKZFrp1BDKg9amtkv5zWnLhuUXN32nhqMvbtMdV2hz7j",
    ])
  }

  func testValidatorNamesMissingFields() {
    let result = AuthNodeInfoValidator.validate(#"{"token":"t","addresses":[]}"#)
    XCTAssertEqual(result, .invalid(
      "missing fields: identity.psk, identity.owner.node_id, identity.owner.user_id, node_info.addresses, node_info.network"
    ))
  }

  func testValidatorRejectsAddressesThatAreNotMultiaddrs() {
    let json = #"{"token":"t","psk":"p","node_id":"n","user_id":"u","network":"testnet","addresses":["192.168.1.1","/"]}"#
    XCTAssertEqual(AuthNodeInfoValidator.validate(json), .invalid("no parseable multiaddr in node_info.addresses"))
  }

  func testValidatorRejectsEmptyAndMalformedPayloads() {
    XCTAssertEqual(AuthNodeInfoValidator.validate(" "), .invalid("empty QR payload"))
    guard case let .invalid(reason) = AuthNodeInfoValidator.validate("{not json") else {
      return XCTFail("expected invalid")
    }
    XCTAssertTrue(reason.hasPrefix("malformed pairing JSON"))
    guard case let .invalid(garbage) = AuthNodeInfoValidator.validate("abc") else {
      return XCTFail("expected invalid")
    }
    XCTAssertTrue(garbage.hasPrefix("malformed pairing payload"))
  }

  func testDialAddrsDropsAddressesTheDeviceCannotReach() {
    let addrs = [
      "/ip4/127.0.0.1/tcp/4001",
      "/ip4/0.0.0.0/tcp/4001",
      "/ip4/169.254.3.4/tcp/4001",
      "/ip6/::1/tcp/4001",
      "/ip6/FE80::1/tcp/4001",
      "/ip6zone/en0/ip6/fe80::1/tcp/4001",
      "/ip4/10.0.0.2/tcp/4001",
      "/dns4/node.example/tcp/4001",
    ]
    XCTAssertEqual(dialAddrs(addrs, peerId: "P"), [
      "/ip4/10.0.0.2/tcp/4001/p2p/P",
      "/dns4/node.example/tcp/4001/p2p/P",
    ])
  }
}
