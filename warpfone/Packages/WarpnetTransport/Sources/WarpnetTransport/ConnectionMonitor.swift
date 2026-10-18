import Foundation
import Observation

@MainActor
@Observable
public final class ConnectionMonitor {
  public static let defaultReconnectBackoffs: [Duration] = [
    .seconds(1),
    .seconds(2),
    .seconds(5),
    .seconds(10),
    .seconds(30),
  ]

  public private(set) var linkState: LinkState = .unknown

  @ObservationIgnored private let client: WarpnetClient
  @ObservationIgnored private let binding: WarpnetBinding
  @ObservationIgnored private let dialAddresses: @Sendable () async -> [String]
  @ObservationIgnored private let pollInterval: Duration
  @ObservationIgnored private let reconnectBackoffs: [Duration]
  @ObservationIgnored private var pollTask: Task<Void, Never>?
  @ObservationIgnored private var reconnectTask: Task<Void, Never>?
  @ObservationIgnored private var reconnectGeneration = 0

  public init(client: WarpnetClient,
              binding: WarpnetBinding,
              pollInterval: Duration = .seconds(2),
              reconnectBackoffs: [Duration] = ConnectionMonitor.defaultReconnectBackoffs,
              dialAddresses: @escaping @Sendable () async -> [String])
  {
    self.client = client
    self.binding = binding
    self.pollInterval = pollInterval
    self.reconnectBackoffs = reconnectBackoffs
    self.dialAddresses = dialAddresses
  }

  public func start() {
    guard pollTask == nil else { return }
    pollTask = Task {
      while !Task.isCancelled {
        await poll()
        try? await Task.sleep(for: pollInterval)
      }
    }
  }

  public func stop() {
    pollTask?.cancel()
    pollTask = nil
    reconnectTask?.cancel()
    reconnectTask = nil
    linkState = .unknown
  }

  func poll() async {
    guard await client.state != .uninitialised else {
      publish(.unknown)
      return
    }
    let binding = binding
    let next = LinkState(binding: await runBlocking { binding.connectedness() })
    guard !Task.isCancelled else { return }
    guard next == .notConnected else {
      publish(next)
      return
    }
    if reconnectTask == nil {
      startReconnect()
    } else if case .reconnecting = linkState {
      return
    } else {
      publish(next)
    }
  }

  private func startReconnect() {
    reconnectGeneration += 1
    let generation = reconnectGeneration
    reconnectTask = Task {
      await reconnectLoop()
      if reconnectGeneration == generation {
        reconnectTask = nil
      }
    }
  }

  private func reconnectLoop() async {
    var attempt = 0
    while !Task.isCancelled {
      attempt += 1
      publish(.reconnecting(attempt: attempt))
      let candidates = await dialAddresses()
      guard !candidates.isEmpty else {
        publish(.notConnected)
        return
      }
      do {
        try await client.connect(candidates)
        return
      } catch {
        let wait = reconnectBackoffs[min(attempt, reconnectBackoffs.count) - 1]
        try? await Task.sleep(for: wait)
      }
    }
  }

  private func publish(_ state: LinkState) {
    if linkState != state {
      linkState = state
    }
  }
}
