import Foundation

public enum WarpnetError: Error, Equatable, Sendable {
  case notInitialised
  case notConnected
  case protocolError(code: Int, message: String)
  case transportFailure(String)
  case serializationError(String)
  case signingUnavailable(String)

  public static let rateLimitedCode = 5001

  public var isRateLimited: Bool {
    if case let .protocolError(code, _) = self {
      return code == Self.rateLimitedCode
    }
    return false
  }

  public var isDurablePairRejection: Bool {
    guard case let .protocolError(_, message) = self else { return false }
    let lowered = message.lowercased()
    return ["token mismatch", "empty token", "pairing revoked", "too many aliases"]
      .contains { lowered.contains($0) }
  }
}

extension WarpnetError: LocalizedError {
  public var errorDescription: String? {
    switch self {
    case .notInitialised:
      "Warpnet transport is not initialised"
    case .notConnected:
      "Not connected to a Warpnet node"
    case let .protocolError(code, message):
      "Warpnet error \(code): \(message)"
    case let .transportFailure(message),
         let .serializationError(message),
         let .signingUnavailable(message):
      message
    }
  }
}
