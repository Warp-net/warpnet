// Modified by Warpnet. See NOTICE.

import Combine
import Env
import Models
import Network
import Observation
import SwiftUI
import WarpnetTransport

@MainActor
@Observable public class AppAccountsManager {
  public private(set) var currentAccount: AppAccount
  public private(set) var currentClient: Client

  public var availableAccounts: [AppAccount] {
    currentClient.isAuth ? [currentAccount] : []
  }

  @ObservationIgnored public private(set) var session: WarpnetSession?

  public static var shared = AppAccountsManager()

  private static let unpairedAccount = AppAccount(server: "", accountName: nil)

  init() {
    currentAccount = Self.unpairedAccount
    currentClient = .init(server: Self.unpairedAccount.server)
  }

  public func configure(session: WarpnetSession) {
    self.session = session
  }

  public func setPaired(_ info: AuthNodeInfo) {
    guard let session else { return }
    currentAccount = AppAccount(server: info.nodeId, accountName: nil)
    currentClient = .init(server: info.nodeId, session: session)
  }

  public func setUnpaired() {
    currentAccount = Self.unpairedAccount
    currentClient = .init(server: Self.unpairedAccount.server)
  }
}
