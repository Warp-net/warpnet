import Foundation
import OSLog
import WarpnetTransport

#if canImport(Warpnet)
  import Warpnet

  struct GoBinding: WarpnetBinding {
    func initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String) -> String {
      NodeInitialize(privKeyHex, warpNetwork, pskHex, bootstrapNodes)
    }

    func connect(addrInfo: String) -> String {
      NodeConnect(addrInfo)
    }

    func stream(protocolId: String, data: String) -> String {
      NodeStream(protocolId, data)
    }

    func peerId() -> String {
      NodePeerID()
    }

    func isConnected() -> Bool {
      NodeIsConnected() == "true"
    }

    func connectedness() -> String {
      NodeConnectedness()
    }

    func disconnect() -> String {
      NodeDisconnect()
    }

    func pause() {
      NodePause()
    }

    func resume() {
      NodeResume()
    }

    func shutdown() -> String {
      NodeShutdown()
    }

    func sign(body: String) -> String {
      NodeSign(body)
    }

    func refreshPeerAddrs(_ addrs: String) -> String {
      NodeRefreshPeerAddrs(addrs)
    }

    func setLogSink(_ sink: WarpnetTransport.NodeLogSink?) {
      NodeSetLogSink(sink.map { GoLogSink($0) })
    }

    func setLogSinkMinLevel(_ level: NodeLogLevel) {
      NodeSetLogSinkMinLevel(level.rawValue)
    }
  }

  private final class GoLogSink: NSObject, NodeLogSinkProtocol {
    private let sink: WarpnetTransport.NodeLogSink

    init(_ sink: WarpnetTransport.NodeLogSink) {
      self.sink = sink
    }

    func write(_ level: Int, component: String?, msg: String?) {
      sink.write(level: NodeLogLevel(rawValue: level) ?? .info, component: component ?? "", message: msg ?? "")
    }
  }
#endif

func makeWarpnetSession() -> WarpnetSession? {
  #if canImport(Warpnet)
    return WarpnetSession(binding: GoBinding(),
                          nodeLogSink: NodeLogger(),
                          keychainService: "\(Bundle.main.bundleIdentifier ?? "warpfone").pairing",
                          accessGroup: nil)
  #else
    return nil
  #endif
}

struct NodeLogger: WarpnetTransport.NodeLogSink {
  private let logger = Logger(subsystem: "site.warpnet.warpfone", category: "node")

  func write(level: NodeLogLevel, component: String, message: String) {
    NodeLogBuffer.shared.append(level: level, component: component, message: message)
    switch level {
    case .debug:
      logger.debug("\(component, privacy: .public): \(message, privacy: .public)")
    case .info:
      logger.info("\(component, privacy: .public): \(message, privacy: .public)")
    case .warn:
      logger.warning("\(component, privacy: .public): \(message, privacy: .public)")
    case .error:
      logger.error("\(component, privacy: .public): \(message, privacy: .public)")
    }
  }
}
