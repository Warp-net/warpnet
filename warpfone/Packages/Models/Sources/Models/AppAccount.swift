// Modified by Warpnet. See NOTICE.

import Foundation
import SwiftUI

public struct AppAccount: Codable, Identifiable, Hashable {
  public let server: String
  public var accountName: String?

  public var key: String {
    server
  }

  public var id: String {
    key
  }

  public init(server: String, accountName: String?) {
    self.server = server
    self.accountName = accountName
  }
}

extension AppAccount: Sendable {}
