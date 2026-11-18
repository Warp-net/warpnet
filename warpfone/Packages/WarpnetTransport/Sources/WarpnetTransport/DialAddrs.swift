private let unreachablePrefixes = [
  "/ip4/127.",
  "/ip4/0.0.0.0/",
  "/ip4/169.254.",
  "/ip6/::1/",
  "/ip6/::/",
  "/ip6/fe80:",
  "/ip6zone/",
]

/// `relays` are the node's bootstrap peers, where it holds its reservations.
/// The announced `addresses` are taken before those exist, so circuits are
/// built here: away from home they are the only way to reach the node.
public func dialAddrs(_ addresses: [String], peerId: String, relays: [String] = []) -> [String] {
  let direct = addresses
    .filter { !isUnreachableFromDevice($0) }
    .map { "\($0)/p2p/\(peerId)" }
  let relayed = relays
    .filter { !$0.hasSuffix("/p2p/\(peerId)") }
    .map { "\($0)/p2p-circuit/p2p/\(peerId)" }
  var seen = Set<String>()
  return (direct + relayed).filter { seen.insert($0).inserted }
}

func isUnreachableFromDevice(_ addr: String) -> Bool {
  let lowered = addr.lowercased()
  return unreachablePrefixes.contains { lowered.hasPrefix($0) }
}
