import Foundation
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
  }
#endif

func makeWarpnetSession() -> WarpnetSession? {
  #if canImport(Warpnet)
    return WarpnetSession(binding: GoBinding(),
                          keychainService: "\(Bundle.main.bundleIdentifier ?? "warpfone").pairing",
                          accessGroup: nil)
  #else
    return nil
  #endif
}
