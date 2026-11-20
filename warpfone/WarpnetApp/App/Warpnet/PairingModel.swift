import AppAccount
import Foundation
import Observation
import WarpnetTransport

@MainActor
@Observable
final class PairingModel {
  enum Phase: Equatable {
    case scanning
    case restoring
    case confirming(AuthNodeInfo, rawJson: String)
    case pairing
    case failed(String, canRetry: Bool)
  }

  static let shared = PairingModel(session: makeWarpnetSession(), accounts: .shared)

  private static let restoreRetries = 3

  private(set) var phase: Phase = .scanning
  var notice: String?

  @ObservationIgnored let session: WarpnetSession?
  @ObservationIgnored let monitor: ConnectionMonitor?
  @ObservationIgnored private let accounts: AppAccountsManager
  @ObservationIgnored private var didRestore = false

  var isAvailable: Bool {
    session != nil
  }

  init(session: WarpnetSession?, accounts: AppAccountsManager) {
    self.session = session
    self.accounts = accounts
    if let session {
      monitor = ConnectionMonitor(client: session.client, binding: session.binding) {
        await session.dialCandidates()
      }
      accounts.configure(session: session)
      Task { [weak self] in
        for await _ in session.client.unpaired {
          await self?.forget()
        }
      }
      if session.storedPairing != nil {
        phase = .restoring
      }
    } else {
      monitor = nil
    }
  }

  func restoreIfNeeded() async {
    guard !didRestore else { return }
    didRestore = true
    await restore()
    if case .failed = phase {
      didRestore = false
    }
  }

  func restore() async {
    guard let session else { return }
    phase = .restoring
    var restored = await session.restore()
    for _ in 0 ..< Self.restoreRetries {
      guard case .transportError = restored else { break }
      try? await Task.sleep(for: .seconds(1))
      guard !Task.isCancelled else { break }
      restored = await session.restore()
    }
    guard let outcome = restored else {
      phase = .scanning
      return
    }
    switch outcome {
    case let .success(info):
      finish(info)
    case .rejected, .peerIdMismatch:
      notice = String(localized: "warpnet.pairing.auto-failed")
      phase = .scanning
    case let .transportError(message):
      phase = .failed(String(localized: "warpnet.pairing.error.transport-\(message)"), canRetry: true)
    }
  }

  func scanned(_ payload: String) {
    guard phase == .scanning else { return }
    switch AuthNodeInfoValidator.validate(payload) {
    case let .valid(info, rawJson):
      notice = nil
      phase = .confirming(info, rawJson: rawJson)
    case let .invalid(reason):
      notice = String(localized: "warpnet.pairing.invalid-\(reason)")
    }
  }

  func confirm() async {
    guard let session, case let .confirming(info, rawJson) = phase else { return }
    phase = .pairing
    switch await session.pair(info, rawJson: rawJson) {
    case let .success(info):
      finish(info)
    case let .rejected(code, message):
      // A string, so the code is not shown with a thousands separator ("5.000").
      phase = .failed(String(localized: "warpnet.pairing.error.rejected-\(String(code))-\(message)"), canRetry: false)
    case .peerIdMismatch:
      phase = .failed(String(localized: "warpnet.pairing.error.peer-mismatch"), canRetry: false)
    case let .transportError(message):
      phase = .failed(String(localized: "warpnet.pairing.error.transport-\(message)"), canRetry: false)
    }
  }

  func cancel() {
    phase = .scanning
  }

  func unpair() async {
    monitor?.stop()
    await session?.unpair()
    accounts.setUnpaired()
    phase = .scanning
  }

  func pause() async {
    guard let session, accounts.currentClient.isAuth else { return }
    monitor?.stop()
    await session.pause()
  }

  func resume() async {
    guard let session, accounts.currentClient.isAuth else { return }
    if case .rejected = await session.resume() {
      accounts.setUnpaired()
      notice = String(localized: "warpnet.pairing.auto-failed")
      phase = .scanning
      return
    }
    monitor?.start()
  }

  private func forget() async {
    guard let session, accounts.currentClient.isAuth else { return }
    monitor?.stop()
    await session.forget()
    accounts.setUnpaired()
    notice = String(localized: "warpnet.pairing.auto-failed")
    phase = .scanning
  }

  private func finish(_ info: AuthNodeInfo) {
    accounts.setPaired(info)
    if UserDefaults.standard.bool(forKey: LogsView.verboseKey), let session {
      Task {
        await session.client.setNodeLogMinLevel(.debug)
      }
    }
    monitor?.start()
    notice = nil
    phase = .scanning
  }
}
