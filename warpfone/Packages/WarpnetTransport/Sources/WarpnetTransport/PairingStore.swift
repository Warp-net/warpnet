import Foundation

public protocol SecretStorage: Sendable {
  func data(for key: String) -> Data?
  func set(_ data: Data, for key: String) throws
  func remove(_ key: String)
}

public protocol IdentitySeedStore: Sendable {
  func seed(memberPeerId: String) throws -> Data
}

public final class PairingStore: IdentitySeedStore, @unchecked Sendable {
  public enum StoreError: Error, Equatable {
    case randomFailed
  }

  static let seedSize = 32
  static let rawQrKey = "paired_node_qr"
  static let identityNodeKey = "identity_member_node"
  static let identitySeedKey = "identity_seed"

  private let storage: SecretStorage
  private let randomBytes: @Sendable (Int) throws -> Data
  private let lock = NSLock()
  private var pendingSeed: (memberPeerId: String, seed: Data)?

  public init(storage: SecretStorage, randomBytes: @escaping @Sendable (Int) throws -> Data) {
    self.storage = storage
    self.randomBytes = randomBytes
  }

  public func loadRawQr() -> String? {
    storage.data(for: Self.rawQrKey).flatMap { String(data: $0, encoding: .utf8) }
  }

  public func save(rawQr: String, memberPeerId: String) throws {
    lock.lock()
    defer { lock.unlock() }
    if let pending = pendingSeed, pending.memberPeerId == memberPeerId {
      try storage.set(Data(memberPeerId.utf8), for: Self.identityNodeKey)
      try storage.set(pending.seed, for: Self.identitySeedKey)
    }
    pendingSeed = nil
    try storage.set(Data(rawQr.utf8), for: Self.rawQrKey)
  }

  public func seed(memberPeerId: String) throws -> Data {
    lock.lock()
    defer { lock.unlock() }
    let storedNode = storage.data(for: Self.identityNodeKey).flatMap { String(data: $0, encoding: .utf8) }
    if storedNode == memberPeerId,
       let stored = storage.data(for: Self.identitySeedKey),
       stored.count == Self.seedSize
    {
      return stored
    }
    if let pending = pendingSeed, pending.memberPeerId == memberPeerId {
      return pending.seed
    }
    let seed = try randomBytes(Self.seedSize)
    guard seed.count == Self.seedSize else { throw StoreError.randomFailed }
    pendingSeed = (memberPeerId, seed)
    return seed
  }

  public func clear() {
    lock.lock()
    defer { lock.unlock() }
    pendingSeed = nil
    storage.remove(Self.rawQrKey)
    storage.remove(Self.identityNodeKey)
    storage.remove(Self.identitySeedKey)
  }
}

#if canImport(Security)
  import Security

  public extension PairingStore {
    static func keychain(service: String, accessGroup: String? = nil) -> PairingStore {
      PairingStore(storage: KeychainStorage(service: service, accessGroup: accessGroup)) { count in
        var bytes = [UInt8](repeating: 0, count: count)
        guard SecRandomCopyBytes(kSecRandomDefault, count, &bytes) == errSecSuccess else {
          throw StoreError.randomFailed
        }
        return Data(bytes)
      }
    }
  }

  public struct KeychainStorage: SecretStorage {
    public struct KeychainError: Error, Equatable {
      public let status: OSStatus
    }

    private let service: String
    private let accessGroup: String?

    public init(service: String, accessGroup: String? = nil) {
      self.service = service
      self.accessGroup = accessGroup
    }

    public func data(for key: String) -> Data? {
      var query = baseQuery(key)
      query[kSecReturnData as String] = true
      query[kSecMatchLimit as String] = kSecMatchLimitOne
      var result: CFTypeRef?
      guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess else { return nil }
      return result as? Data
    }

    public func set(_ data: Data, for key: String) throws {
      let query = baseQuery(key)
      let attributes: [String: Any] = [
        kSecValueData as String: data,
        kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
      ]
      var status = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
      if status == errSecItemNotFound {
        status = SecItemAdd(query.merging(attributes) { $1 } as CFDictionary, nil)
      }
      guard status == errSecSuccess else { throw KeychainError(status: status) }
    }

    public func remove(_ key: String) {
      SecItemDelete(baseQuery(key) as CFDictionary)
    }

    private func baseQuery(_ key: String) -> [String: Any] {
      var query: [String: Any] = [
        kSecClass as String: kSecClassGenericPassword,
        kSecAttrService as String: service,
        kSecAttrAccount as String: key,
        kSecUseDataProtectionKeychain as String: true,
      ]
      if let accessGroup {
        query[kSecAttrAccessGroup as String] = accessGroup
      }
      return query
    }
  }
#endif
