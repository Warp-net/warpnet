import Foundation
@testable import WarpnetTransport
import XCTest

final class PairingStoreTests: XCTestCase {
  func makeStore(_ storage: MemoryStorage) -> PairingStore {
    PairingStore(storage: storage) { count in
      Data((0 ..< count).map { _ in UInt8.random(in: 0 ... 255) })
    }
  }

  func testCandidateSeedStaysInMemoryUntilThePairingSucceeds() throws {
    let storage = MemoryStorage()
    let store = makeStore(storage)
    let seed = try store.seed(memberPeerId: "A")
    XCTAssertEqual(seed.count, 32)
    XCTAssertEqual(try store.seed(memberPeerId: "A"), seed)
    XCTAssertNil(storage.data(for: PairingStore.identitySeedKey))

    try store.save(rawQr: "{\"node_id\":\"A\"}", memberPeerId: "A")
    XCTAssertEqual(storage.data(for: PairingStore.identitySeedKey), seed)
    XCTAssertEqual(store.loadRawQr(), "{\"node_id\":\"A\"}")
    XCTAssertEqual(try makeStore(storage).seed(memberPeerId: "A"), seed)
  }

  func testPairingAnotherNodeKeepsTheSavedIdentityUntilItSucceeds() throws {
    let storage = MemoryStorage()
    let store = makeStore(storage)
    let first = try store.seed(memberPeerId: "A")
    try store.save(rawQr: "a", memberPeerId: "A")

    let candidate = try store.seed(memberPeerId: "B")
    XCTAssertNotEqual(candidate, first)
    XCTAssertEqual(try store.seed(memberPeerId: "A"), first)

    try store.save(rawQr: "b", memberPeerId: "B")
    XCTAssertEqual(try store.seed(memberPeerId: "B"), candidate)
    XCTAssertNotEqual(try store.seed(memberPeerId: "A"), first)
  }

  func testClearForgetsTheIdentity() throws {
    let storage = MemoryStorage()
    let store = makeStore(storage)
    let seed = try store.seed(memberPeerId: "A")
    try store.save(rawQr: "a", memberPeerId: "A")
    store.clear()
    XCTAssertNil(store.loadRawQr())
    XCTAssertNil(storage.data(for: PairingStore.identitySeedKey))
    XCTAssertNotEqual(try store.seed(memberPeerId: "A"), seed)
  }

  func testShortRandomOutputIsRejected() {
    let store = PairingStore(storage: MemoryStorage()) { _ in Data([1, 2, 3]) }
    XCTAssertThrowsError(try store.seed(memberPeerId: "A")) {
      XCTAssertEqual($0 as? PairingStore.StoreError, .randomFailed)
    }
  }

  #if canImport(CryptoKit)
    func testPrivateKeyIsSeedThenPublicKey() throws {
      let seed = Data(hex: "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
      let key = try DeviceIdentity.privateKey(seed: seed)
      XCTAssertEqual(key.hexString, "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
        + "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
    }

    func testPrivateKeyHexUsesTheStoredSeed() throws {
      let store = makeStore(MemoryStorage())
      let hex = try DeviceIdentity.privateKeyHex(memberPeerId: "A", seeds: store)
      XCTAssertEqual(hex.count, 128)
      XCTAssertTrue(hex.hasPrefix(try store.seed(memberPeerId: "A").hexString))
      XCTAssertThrowsError(try DeviceIdentity.privateKeyHex(memberPeerId: "", seeds: store))
      XCTAssertThrowsError(try DeviceIdentity.privateKey(seed: Data([1])))
    }
  #endif
}

extension Data {
  init(hex: String) {
    var bytes: [UInt8] = []
    var index = hex.startIndex
    while index < hex.endIndex {
      let next = hex.index(index, offsetBy: 2)
      bytes.append(UInt8(hex[index ..< next], radix: 16)!)
      index = next
    }
    self.init(bytes)
  }
}
