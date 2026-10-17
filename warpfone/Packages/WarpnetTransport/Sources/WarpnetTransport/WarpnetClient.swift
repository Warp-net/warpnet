import Foundation

public actor WarpnetClient {
  public static let defaultRetryBackoffs: [Duration] = [
    .milliseconds(500),
    .milliseconds(1500),
    .seconds(4),
    .seconds(8),
  ]

  public private(set) var state: ConnectionState = .uninitialised

  private let binding: WarpnetBinding
  private let nodeLogSink: NodeLogSink?
  private let retryBackoffs: [Duration]
  private var isControlBusy = false
  private var controlWaiters: [CheckedContinuation<Void, Never>] = []

  public init(binding: WarpnetBinding,
              nodeLogSink: NodeLogSink? = nil,
              retryBackoffs: [Duration] = WarpnetClient.defaultRetryBackoffs)
  {
    self.binding = binding
    self.nodeLogSink = nodeLogSink
    self.retryBackoffs = retryBackoffs
  }

  public func initialise(_ config: WarpnetConfig) async throws {
    await lockControl()
    defer { unlockControl() }
    guard state == .uninitialised else { return }
    guard !config.bootstrapAddrs.isEmpty else {
      throw WarpnetError.transportFailure("bootstrap nodes required")
    }
    let binding = binding
    let sink = nodeLogSink
    let failure = await runBlocking {
      let err = binding.initialize(privKeyHex: config.privKeyHex,
                                   warpNetwork: config.network,
                                   pskHex: config.pskHex,
                                   bootstrapNodes: config.bootstrapAddrs.joined(separator: ","))
      if err.isEmpty, let sink {
        binding.setLogSink(sink)
      }
      return err
    }
    guard failure.isEmpty else {
      let error = WarpnetError.transportFailure(failure)
      state = .failed(error)
      throw error
    }
    state = .disconnected
  }

  public func setNodeLogMinLevel(_ level: NodeLogLevel) async {
    let binding = binding
    await runBlocking { binding.setLogSinkMinLevel(level) }
  }

  public func connect(_ candidateAddrs: [String]) async throws {
    guard !candidateAddrs.isEmpty else {
      throw WarpnetError.transportFailure("no addresses to dial")
    }
    await lockControl()
    defer { unlockControl() }
    guard state != .uninitialised else { throw WarpnetError.notInitialised }
    state = .connecting
    let binding = binding
    let joined = candidateAddrs.joined(separator: "\n")
    let failure = await runBlocking { binding.connect(addrInfo: joined) }
    guard failure.isEmpty else {
      let error = WarpnetError.transportFailure(
        "\(failure). On LAN this is most often the PC firewall blocking the libp2p port."
      )
      state = .failed(error)
      throw error
    }
    state = .connected
  }

  public func pair(_ rawAuthNodeInfoJson: String) async throws {
    await lockControl()
    defer { unlockControl() }
    guard state == .connected else { throw WarpnetError.notConnected }

    let raw = try await send(protocolId: ProtocolIds.PRIVATE_POST_PAIR, body: rawAuthNodeInfoJson)
    let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else {
      throw WarpnetError.transportFailure("empty pairing response")
    }
    if trimmed.hasPrefix("{") {
      if let parsed = try? JSONDecoder().decode(WarpnetResponseError.self, from: Data(trimmed.utf8)),
         parsed.code != 0 || !parsed.message.isEmpty
      {
        throw WarpnetError.protocolError(code: parsed.code, message: parsed.message)
      }
      throw WarpnetError.transportFailure(trimmed)
    }
    guard let addrs = try? JSONDecoder().decode([String].self, from: Data(trimmed.utf8)) else {
      throw WarpnetError.transportFailure("unexpected pairing response: \(trimmed)")
    }
    guard !addrs.isEmpty else { return }
    let binding = binding
    let joined = addrs.joined(separator: "\n")
    let failure = await runBlocking { binding.refreshPeerAddrs(joined) }
    guard failure.isEmpty else { throw WarpnetError.transportFailure(failure) }
  }

  public func request(_ protocolId: String, body: String) async throws -> String {
    var attempt = 0
    while true {
      guard state == .connected else { throw WarpnetError.notConnected }
      do {
        let raw = try await send(protocolId: protocolId, body: body)
        try Self.throwIfErrorResponse(raw)
        return raw
      } catch let error as WarpnetError {
        guard attempt < retryBackoffs.count, Self.isRetryable(error) else { throw error }
        try await Task.sleep(for: retryBackoffs[attempt])
        attempt += 1
      }
    }
  }

  public func pause() async {
    await lockControl()
    defer { unlockControl() }
    guard state != .uninitialised else { return }
    let binding = binding
    await runBlocking { binding.pause() }
    if state == .connected || state == .connecting {
      state = .disconnected
    }
  }

  public func resume() async {
    await lockControl()
    defer { unlockControl() }
    guard state != .uninitialised else { return }
    let binding = binding
    let isConnected = await runBlocking {
      binding.resume()
      return binding.isConnected()
    }
    if case .failed = state { return }
    state = isConnected ? .connected : .disconnected
  }

  public func shutdown() async {
    await lockControl()
    defer { unlockControl() }
    let binding = binding
    _ = await runBlocking { binding.shutdown() }
    state = .uninitialised
  }

  private func send(protocolId: String, body: String) async throws -> String {
    let binding = binding
    let result: Result<String, WarpnetError> = await runBlocking {
      var envelope = WarpnetEnvelope(body: body,
                                     nodeId: binding.peerId(),
                                     path: protocolId,
                                     timestampNanos: WarpnetTimestamp.nowNanos())
      let signature = binding.sign(body: envelope.signingInput)
      guard !signature.isEmpty, !signature.hasPrefix("error:") else {
        return .failure(.signingUnavailable(signature.isEmpty ? "binding not initialised" : signature))
      }
      envelope.signature = signature
      return .success(binding.stream(protocolId: protocolId, data: envelope.json()))
    }
    return try result.get()
  }

  static func throwIfErrorResponse(_ raw: String) throws {
    if raw.isEmpty { return }
    guard let first = raw.first(where: { !$0.isWhitespace }) else {
      throw WarpnetError.transportFailure(raw)
    }
    if first == "[" { return }
    guard first == "{" else { throw WarpnetError.transportFailure(raw) }
    guard let error = try? JSONDecoder().decode(WarpnetResponseError.self, from: Data(raw.utf8)) else {
      return
    }
    if error.code == 0 { return }
    throw WarpnetError.protocolError(code: error.code, message: error.message)
  }

  static func isRetryable(_ error: WarpnetError) -> Bool {
    switch error {
    case let .transportFailure(message):
      isRetryableTransportError(message)
    case .protocolError:
      error.isRateLimited
    default:
      false
    }
  }

  static func isRetryableTransportError(_ message: String) -> Bool {
    let lowered = message.lowercased()
    if lowered.contains("stream: reading response") { return false }
    return [
      "failed to open stream",
      "not connected to desktop node",
      "context deadline exceeded",
      "stream reset",
      "muxer closed",
    ].contains { lowered.contains($0) }
  }

  private func lockControl() async {
    guard isControlBusy else {
      isControlBusy = true
      return
    }
    await withCheckedContinuation { controlWaiters.append($0) }
  }

  private func unlockControl() {
    if controlWaiters.isEmpty {
      isControlBusy = false
    } else {
      controlWaiters.removeFirst().resume()
    }
  }
}

func runBlocking<T: Sendable>(_ work: @escaping @Sendable () -> T) async -> T {
  await withCheckedContinuation { continuation in
    DispatchQueue.global(qos: .userInitiated).async {
      continuation.resume(returning: work())
    }
  }
}
