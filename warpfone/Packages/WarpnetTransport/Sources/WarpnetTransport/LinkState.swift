public enum LinkState: Equatable, Sendable {
  case unknown
  case connected
  case limited
  case notConnected
  case reconnecting(attempt: Int)

  public var isUp: Bool {
    self == .connected || self == .limited
  }

  public init(binding raw: String) {
    switch raw {
    case "Connected":
      self = .connected
    case "Limited":
      self = .limited
    case "NotConnected", "CanConnect", "CannotConnect":
      self = .notConnected
    default:
      self = .unknown
    }
  }
}
