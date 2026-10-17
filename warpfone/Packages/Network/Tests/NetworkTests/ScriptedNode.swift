import Foundation
@testable import Network
import WarpnetTransport

final class ScriptedBinding: WarpnetBinding, @unchecked Sendable {
  typealias Handler = @Sendable (_ route: String, _ body: [String: Any]) -> String

  private let lock = NSLock()
  private let handler: Handler
  private var recorded: [(route: String, body: [String: Any])] = []

  init(handler: @escaping Handler) {
    self.handler = handler
  }

  var calls: [(route: String, body: [String: Any])] {
    lock.lock()
    defer { lock.unlock() }
    return recorded
  }

  func calls(to route: String) -> [[String: Any]] {
    calls.filter { $0.route == route }.map(\.body)
  }

  func initialize(privKeyHex _: String, warpNetwork _: String, pskHex _: String, bootstrapNodes _: String) -> String { "" }
  func connect(addrInfo _: String) -> String { "" }

  func stream(protocolId: String, data: String) -> String {
    let envelope = (try? JSONSerialization.jsonObject(with: Data(data.utf8))) as? [String: Any] ?? [:]
    let body = envelope["body"] as? [String: Any] ?? [:]
    lock.lock()
    recorded.append((protocolId, body))
    lock.unlock()
    if protocolId == ProtocolIds.PRIVATE_POST_PAIR {
      return "[]"
    }
    return handler(protocolId, body)
  }

  func peerId() -> String { "12D3KooWDevice" }
  func isConnected() -> Bool { true }
  func connectedness() -> String { "Connected" }
  func disconnect() -> String { "" }
  func pause() {}
  func resume() {}
  func shutdown() -> String { "" }
  func sign(body _: String) -> String { "c2ln" }
  func refreshPeerAddrs(_: String) -> String { "" }
  func setLogSink(_: NodeLogSink?) {}
  func setLogSinkMinLevel(_: NodeLogLevel) {}
}

private final class MemoryStorage: SecretStorage, @unchecked Sendable {
  private let lock = NSLock()
  private var values: [String: Data] = [:]

  func data(for key: String) -> Data? {
    lock.lock()
    defer { lock.unlock() }
    return values[key]
  }

  func set(_ data: Data, for key: String) throws {
    lock.lock()
    defer { lock.unlock() }
    values[key] = data
  }

  func remove(_ key: String) {
    lock.lock()
    defer { lock.unlock() }
    values[key] = nil
  }
}

enum ScriptedNode {
  static let viewerId = "VIEWER"
  static let nodeId = "12D3KooWNode"

  static func pairedSession(_ binding: ScriptedBinding) async -> WarpnetSession {
    let session = WarpnetSession(binding: binding,
                                 client: WarpnetClient(binding: binding, retryBackoffs: []),
                                 store: PairingStore(storage: MemoryStorage()) { Data(repeating: 1, count: $0) },
                                 derivePrivateKeyHex: { _, _ in "00" })
    let info = AuthNodeInfo(token: "t",
                            psk: "00",
                            network: "testnet",
                            addresses: ["/ip4/10.0.0.2/tcp/4001"],
                            nodeId: nodeId,
                            userId: viewerId,
                            bootstrapPeers: [])
    _ = await session.pair(info, rawJson: "{}")
    return session
  }

  static func json(_ object: Any) -> String {
    let data = (try? JSONSerialization.data(withJSONObject: object, options: [.withoutEscapingSlashes])) ?? Data()
    return String(decoding: data, as: UTF8.self)
  }

  static func user(_ id: String, _ name: String, avatar: String? = nil) -> [String: Any] {
    var user: [String: Any] = [
      "id": id,
      "username": name,
      "bio": "Hello & welcome",
      "created_at": "2025-01-02T03:04:05Z",
      "followers_count": 3,
      "followings_count": 4,
      "tweets_count": 5,
      "node_id": "12D3KooW\(id)",
      "network": "testnet",
    ]
    if let avatar {
      user["avatar_key"] = avatar
    }
    return user
  }

  static func tweet(_ id: String,
                    by userId: String,
                    text: String = "hello",
                    createdAt: String = "2026-10-17T08:29:00.123456789Z",
                    extra: [String: Any] = [:]) -> [String: Any]
  {
    var tweet: [String: Any] = [
      "id": id,
      "root_id": id,
      "text": text,
      "user_id": userId,
      "username": "name-\(userId)",
      "created_at": createdAt,
      "network": "testnet",
    ]
    tweet.merge(extra) { _, new in new }
    return tweet
  }
}
