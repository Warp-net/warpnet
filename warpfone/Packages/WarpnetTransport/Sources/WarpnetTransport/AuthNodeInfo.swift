import Foundation

public struct AuthNodeInfo: Codable, Equatable, Sendable {
  public let token: String
  public let psk: String
  public let network: String
  public let addresses: [String]
  public let nodeId: String
  public let userId: String
  public let bootstrapPeers: [String]

  enum CodingKeys: String, CodingKey {
    case token
    case psk
    case network
    case addresses
    case nodeId = "node_id"
    case userId = "user_id"
    case bootstrapPeers = "bootstrap_peers"
  }

  public init(token: String,
              psk: String,
              network: String,
              addresses: [String],
              nodeId: String,
              userId: String,
              bootstrapPeers: [String])
  {
    self.token = token
    self.psk = psk
    self.network = network
    self.addresses = addresses
    self.nodeId = nodeId
    self.userId = userId
    self.bootstrapPeers = bootstrapPeers
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    token = try c.decodeIfPresent(String.self, forKey: .token) ?? ""
    psk = try c.decodeIfPresent(String.self, forKey: .psk) ?? ""
    network = try c.decodeIfPresent(String.self, forKey: .network) ?? ""
    addresses = try c.decodeIfPresent([String].self, forKey: .addresses) ?? []
    nodeId = try c.decodeIfPresent(String.self, forKey: .nodeId) ?? ""
    userId = try c.decodeIfPresent(String.self, forKey: .userId) ?? ""
    bootstrapPeers = try c.decodeIfPresent([String].self, forKey: .bootstrapPeers) ?? []
  }

  public var dialCandidates: [String] {
    dialAddrs(addresses, peerId: nodeId)
  }
}

public enum AuthNodeInfoValidation: Equatable, Sendable {
  case valid(AuthNodeInfo, rawJson: String)
  case invalid(String)
}

public enum AuthNodeInfoValidator {
  public static func validate(_ rawPayload: String) -> AuthNodeInfoValidation {
    guard !rawPayload.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
      return .invalid("empty QR payload")
    }
    let rawJson: String
    do {
      rawJson = try QrPayloadCodec.decode(rawPayload)
    } catch {
      return .invalid("malformed pairing payload: \(error)")
    }
    guard !rawJson.isEmpty else { return .invalid("empty pairing payload") }

    let info: AuthNodeInfo
    do {
      info = try JSONDecoder().decode(AuthNodeInfo.self, from: Data(rawJson.utf8))
    } catch {
      return .invalid("malformed pairing JSON: \(error.localizedDescription)")
    }

    var missing: [String] = []
    if info.token.isBlank { missing.append("identity.token") }
    if info.psk.isBlank { missing.append("identity.psk") }
    if info.nodeId.isBlank { missing.append("identity.owner.node_id") }
    if info.userId.isBlank { missing.append("identity.owner.user_id") }
    if info.addresses.isEmpty { missing.append("node_info.addresses") }
    if info.network.isBlank { missing.append("node_info.network") }
    guard missing.isEmpty else {
      return .invalid("missing fields: \(missing.joined(separator: ", "))")
    }

    let hasDialable = info.addresses.contains {
      $0.hasPrefix("/") && !$0.trimmingCharacters(in: CharacterSet(charactersIn: "/")).isEmpty
    }
    guard hasDialable else {
      return .invalid("no parseable multiaddr in node_info.addresses")
    }
    return .valid(info, rawJson: rawJson)
  }
}

private extension String {
  var isBlank: Bool {
    trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
  }
}
