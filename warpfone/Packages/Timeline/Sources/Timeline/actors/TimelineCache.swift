// Modified by Warpnet. See NOTICE.

import Models
import Network
import SwiftUI

public actor TimelineCache {
  public init() {}

  func setLatestSeenStatuses(_ statuses: [Status], for client: Client, filter: String) {
    let statuses = statuses.sorted(by: { $0.createdAt.asDate > $1.createdAt.asDate })
    if filter == "Home" {
      UserDefaults.standard.set(statuses.map{ $0.id }, forKey: "timeline-last-seen-\(client.id)")
    } else {
      UserDefaults.standard.set(statuses.map{ $0.id }, forKey: "timeline-last-seen-\(client.id)-\(filter)")
    }
  }

  func getLatestSeenStatus(for client: Client, filter: String) -> [String]? {
    if filter == "Home" {
      UserDefaults.standard.array(forKey: "timeline-last-seen-\(client.id)") as? [String]
    } else {
      UserDefaults.standard.array(forKey: "timeline-last-seen-\(client.id)-\(filter)") as? [String]
    }
  }
}

