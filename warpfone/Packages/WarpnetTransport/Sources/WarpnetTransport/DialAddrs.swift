private let unreachablePrefixes = [
  "/ip4/127.",
  "/ip4/0.0.0.0/",
  "/ip4/169.254.",
  "/ip6/::1/",
  "/ip6/::/",
  "/ip6/fe80:",
  "/ip6zone/",
]

public func dialAddrs(_ addresses: [String], peerId: String) -> [String] {
  addresses
    .filter { !isUnreachableFromDevice($0) }
    .map { "\($0)/p2p/\(peerId)" }
}

func isUnreachableFromDevice(_ addr: String) -> Bool {
  let lowered = addr.lowercased()
  return unreachablePrefixes.contains { lowered.hasPrefix($0) }
}
