import Foundation

#if canImport(CryptoKit)
  import CryptoKit

  public enum DeviceIdentity {
    public enum IdentityError: Error, Equatable {
      case emptyMemberPeerId
      case unexpectedSeedLength(Int)
    }

    public static func privateKeyHex(memberPeerId: String, seeds: IdentitySeedStore) throws -> String {
      guard !memberPeerId.isEmpty else { throw IdentityError.emptyMemberPeerId }
      return try privateKey(seed: seeds.seed(memberPeerId: memberPeerId)).hexString
    }

    public static func privateKey(seed: Data) throws -> Data {
      guard seed.count == 32 else { throw IdentityError.unexpectedSeedLength(seed.count) }
      let key = try Curve25519.Signing.PrivateKey(rawRepresentation: seed)
      return seed + key.publicKey.rawRepresentation
    }
  }
#endif

extension Data {
  var hexString: String {
    map { byte in
      let hex = String(byte, radix: 16)
      return byte < 0x10 ? "0" + hex : hex
    }.joined()
  }
}
