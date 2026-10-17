import Foundation
@testable import WarpnetTransport

final class FakeBinding: WarpnetBinding, @unchecked Sendable {
  private let lock = NSLock()
  private var _initializeResult = ""
  private var _connectResults: [String] = []
  private var _streamResults: [String] = []
  private var _signResult = "c2lnbmF0dXJl"
  private var _connectedness: [String] = []
  private var _isConnected = true
  private var _linkUp = true
  private var _refreshResult = ""
  private var _streamCalls: [(protocolId: String, data: String)] = []
  private var _signInputs: [String] = []
  private var _connectCalls: [String] = []
  private var _refreshCalls: [String] = []
  private var _pauseCount = 0
  private var _resumeCount = 0
  private var _shutdownCount = 0
  private var _initializeCalls: [(privKeyHex: String, network: String, psk: String, bootstrap: String)] = []

  let peerIdValue = "12D3KooWTestPeer"

  private func locked<T>(_ body: () -> T) -> T {
    lock.lock()
    defer { lock.unlock() }
    return body()
  }

  var initializeResult: String {
    get { locked { _initializeResult } }
    set { locked { _initializeResult = newValue } }
  }

  var connectResults: [String] {
    get { locked { _connectResults } }
    set { locked { _connectResults = newValue } }
  }

  var streamResults: [String] {
    get { locked { _streamResults } }
    set { locked { _streamResults = newValue } }
  }

  var signResult: String {
    get { locked { _signResult } }
    set { locked { _signResult = newValue } }
  }

  var connectednessResults: [String] {
    get { locked { _connectedness } }
    set { locked { _connectedness = newValue } }
  }

  var linkUp: Bool {
    get { locked { _linkUp } }
    set { locked { _linkUp = newValue } }
  }

  var isConnectedResult: Bool {
    get { locked { _isConnected } }
    set { locked { _isConnected = newValue } }
  }

  var refreshResult: String {
    get { locked { _refreshResult } }
    set { locked { _refreshResult = newValue } }
  }

  var streamCalls: [(protocolId: String, data: String)] { locked { _streamCalls } }
  var signInputs: [String] { locked { _signInputs } }
  var connectCalls: [String] { locked { _connectCalls } }
  var refreshCalls: [String] { locked { _refreshCalls } }
  var pauseCount: Int { locked { _pauseCount } }
  var resumeCount: Int { locked { _resumeCount } }
  var shutdownCount: Int { locked { _shutdownCount } }
  var initializeCalls: [(privKeyHex: String, network: String, psk: String, bootstrap: String)] {
    locked { _initializeCalls }
  }

  func initialize(privKeyHex: String, warpNetwork: String, pskHex: String, bootstrapNodes: String) -> String {
    locked {
      _initializeCalls.append((privKeyHex, warpNetwork, pskHex, bootstrapNodes))
      return _initializeResult
    }
  }

  func connect(addrInfo: String) -> String {
    locked {
      _connectCalls.append(addrInfo)
      let result = _connectResults.isEmpty ? "" : _connectResults.removeFirst()
      if result.isEmpty {
        _linkUp = true
      }
      return result
    }
  }

  func stream(protocolId: String, data: String) -> String {
    locked {
      _streamCalls.append((protocolId, data))
      return _streamResults.isEmpty ? "{}" : _streamResults.removeFirst()
    }
  }

  func peerId() -> String {
    peerIdValue
  }

  func isConnected() -> Bool {
    isConnectedResult
  }

  func connectedness() -> String {
    locked {
      guard _connectedness.isEmpty else { return _connectedness.removeFirst() }
      return _linkUp ? "Connected" : "NotConnected"
    }
  }

  func disconnect() -> String {
    ""
  }

  func pause() {
    locked { _pauseCount += 1 }
  }

  func resume() {
    locked { _resumeCount += 1 }
  }

  func shutdown() -> String {
    locked {
      _shutdownCount += 1
      return ""
    }
  }

  func sign(body: String) -> String {
    locked {
      _signInputs.append(body)
      return _signResult
    }
  }

  func refreshPeerAddrs(_ addrs: String) -> String {
    locked {
      _refreshCalls.append(addrs)
      return _refreshResult
    }
  }

  func setLogSink(_: NodeLogSink?) {}

  func setLogSinkMinLevel(_: NodeLogLevel) {}
}

final class MemoryStorage: SecretStorage, @unchecked Sendable {
  private let lock = NSLock()
  private var values: [String: Data] = [:]

  func data(for key: String) -> Data? {
    lock.lock()
    defer { lock.unlock() }
    return values[key]
  }

  func set(_ data: Data, for key: String) throws {
    lock.lock()
    defer { lock.unlock() }
    values[key] = data
  }

  func remove(_ key: String) {
    lock.lock()
    defer { lock.unlock() }
    values[key] = nil
  }
}
