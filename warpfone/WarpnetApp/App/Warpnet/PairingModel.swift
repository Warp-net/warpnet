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
    guard let outcome = await session.restore() else {
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
      phase = .failed(String(localized: "warpnet.pairing.error.rejected-\(code)-\(message)"), canRetry: false)
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

  private func finish(_ info: AuthNodeInfo) {
    accounts.setPaired(info)
    monitor?.start()
    notice = nil
    phase = .scanning
  }
}
