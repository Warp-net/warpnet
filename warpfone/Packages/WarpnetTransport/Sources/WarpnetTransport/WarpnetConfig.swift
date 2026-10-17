public struct WarpnetConfig: Equatable, Sendable {
  public let privKeyHex: String
  public let pskHex: String
  public let bootstrapAddrs: [String]
  public let network: String

  public init(privKeyHex: String, pskHex: String, bootstrapAddrs: [String], network: String) {
    self.privKeyHex = privKeyHex
    self.pskHex = pskHex
    self.bootstrapAddrs = bootstrapAddrs
    self.network = network
  }
}
