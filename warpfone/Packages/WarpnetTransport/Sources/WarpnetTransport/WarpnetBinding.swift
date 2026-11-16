import Foundation

public protocol WarpnetBinding: Sendable {
  func initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String) -> String
  func connect(addrInfo: String) -> String
  func stream(protocolId: String, data: String) -> String
  func peerId() -> String
  func isConnected() -> Bool
  func connectedness() -> String
  func disconnect() -> String
  func pause()
  func resume()
  func shutdown() -> String
  func sign(body: String) -> String
  func refreshPeerAddrs(_ addrs: String) -> String
  func setLogSink(_ sink: NodeLogSink?)
  func setLogSinkMinLevel(_ level: NodeLogLevel)
  /// Like `stream`, but the response, or the error text, stays bytes; see
  /// warpfone/node/mobile.go:Stream.
  func streamBytes(protocolId: String, data: String) -> Data
}

public extension WarpnetBinding {
  func streamBytes(protocolId: String, data: String) -> Data {
    Data(stream(protocolId: protocolId, data: data).utf8)
  }
}
