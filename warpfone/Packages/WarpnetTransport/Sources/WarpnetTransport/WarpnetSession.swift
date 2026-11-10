import Foundation

public enum PairingOutcome: Equatable, Sendable {
  case success(AuthNodeInfo)
  case transportError(String)
  case rejected(code: Int, message: String)
  case peerIdMismatch(expected: String, message: String)
}

public actor WarpnetSession {
  public typealias KeyDerivation = @Sendable (_ memberPeerId: String, _ seeds: IdentitySeedStore) throws -> String

  public nonisolated let binding: WarpnetBinding
  public nonisolated let client: WarpnetClient
  public nonisolated let store: PairingStore
  private let derivePrivateKeyHex: KeyDerivation
  private let repairInterval: TimeInterval
  private var pairedAt: Date?

  public private(set) var pairing: AuthNodeInfo?

  // The node forgets a device that has not paired for 72 hours, and iOS can
  // keep a suspended app alive longer than that.
  public init(binding: WarpnetBinding,
              client: WarpnetClient,
              store: PairingStore,
              repairInterval: TimeInterval = 3600,
              derivePrivateKeyHex: @escaping KeyDerivation)
  {
    self.binding = binding
    self.client = client
    self.store = store
    self.repairInterval = repairInterval
    self.derivePrivateKeyHex = derivePrivateKeyHex
  }

  public nonisolated var storedPairing: AuthNodeInfo? {
    guard let raw = store.loadRawQr(),
          case let .valid(info, _) = AuthNodeInfoValidator.validate(raw)
    else { return nil }
    return info
  }

  public func restore() async -> PairingOutcome? {
    guard let raw = store.loadRawQr() else { return nil }
    guard case let .valid(info, rawJson) = AuthNodeInfoValidator.validate(raw) else {
      store.clear()
      return nil
    }
    let outcome = await pair(info, rawJson: rawJson)
    switch outcome {
    case .rejected, .peerIdMismatch:
      store.clear()
    case .success, .transportError:
      break
    }
    return outcome
  }

  public func pair(_ info: AuthNodeInfo, rawJson: String) async -> PairingOutcome {
    let candidates = info.dialCandidates
    let bootstrap = info.bootstrapPeers.isEmpty ? candidates : info.bootstrapPeers
    do {
      let privKeyHex = try derivePrivateKeyHex(info.nodeId, store)
      let config = WarpnetConfig(privKeyHex: privKeyHex,
                                 pskHex: info.psk,
                                 bootstrapAddrs: bootstrap,
                                 network: info.network)
      if await client.state != .uninitialised {
        await client.shutdown()
      }
      try await client.initialise(config)
      try await client.connect(candidates)
      try await client.pair(rawJson)
      try? store.save(rawQr: rawJson, memberPeerId: info.nodeId)
      pairing = info
      pairedAt = Date()
      return .success(info)
    } catch let error as WarpnetError {
      switch error {
      case let .protocolError(code, message):
        return error.isDurablePairRejection ? .rejected(code: code, message: message) : .transportError(message)
      case let .transportFailure(message) where message.lowercased().contains("peer id mismatch"):
        return .peerIdMismatch(expected: info.nodeId, message: message)
      default:
        return .transportError(error.localizedDescription)
      }
    } catch {
      return .transportError(error.localizedDescription)
    }
  }

  public func unpair() async {
    if pairing != nil {
      let peerId = await client.peerId()
      if let body = try? WarpnetJSON.encode(DeleteDeviceEvent(nodeId: peerId)) {
        _ = try? await client.request(ProtocolIds.PRIVATE_DELETE_DEVICE, body: body)
      }
    }
    await client.shutdown()
    store.clear()
    pairing = nil
    pairedAt = nil
  }

  public func pause() async {
    await client.pause()
  }

  @discardableResult
  public func resume() async -> PairingOutcome? {
    await client.resume()
    guard let pairing, let pairedAt, Date().timeIntervalSince(pairedAt) >= repairInterval,
          let rawJson = store.loadRawQr()
    else { return nil }
    do {
      try await client.pair(rawJson)
      self.pairedAt = Date()
      return .success(pairing)
    } catch let error as WarpnetError where error.isDurablePairRejection {
      await client.shutdown()
      store.clear()
      self.pairing = nil
      self.pairedAt = nil
      guard case let .protocolError(code, message) = error else { return nil }
      return .rejected(code: code, message: message)
    } catch {
      return nil
    }
  }

  public func dialCandidates() -> [String] {
    pairing?.dialCandidates ?? []
  }
}

#if canImport(CryptoKit) && canImport(Security)
  public extension WarpnetSession {
    init(binding: WarpnetBinding, nodeLogSink: NodeLogSink?, keychainService: String, accessGroup: String?) {
      self.init(binding: binding,
                client: WarpnetClient(binding: binding, nodeLogSink: nodeLogSink),
                store: .keychain(service: keychainService, accessGroup: accessGroup),
                derivePrivateKeyHex: { try DeviceIdentity.privateKeyHex(memberPeerId: $0, seeds: $1) })
    }
  }
#endif
