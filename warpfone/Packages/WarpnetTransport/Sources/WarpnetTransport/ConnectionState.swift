public enum ConnectionState: Equatable, Sendable {
  case uninitialised
  case disconnected
  case connecting
  case connected
  case failed(WarpnetError)
}
