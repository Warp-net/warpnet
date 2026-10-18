// Modified by Warpnet. See NOTICE.

import DesignSystem
import Models
import SwiftUI

extension Models.Notification.NotificationType {
  public func label(count: Int) -> LocalizedStringKey {
    switch self {
    case .mention:
      ""
    case .reblog:
      "notifications.label.reblog \(count)"
    case .follow:
      "notifications.label.follow \(count)"
    case .favourite:
      "notifications.label.favorite \(count)"
    case .message:
      "warpnet.notifications.label.message"
    case .moderation:
      "warpnet.notifications.label.moderation"
    }
  }

  var icon: Image {
    switch self {
    case .mention:
      Image(systemName: "at")
    case .reblog:
      Image(systemName: "arrow.2.squarepath")
    case .follow:
      Image(systemName: "person.fill.badge.plus")
    case .favourite:
      Image(systemName: "heart.fill")
    case .message:
      Image(systemName: "envelope.fill")
    case .moderation:
      Image(systemName: "hammer.fill")
    }
  }

  var tintColor: Color {
    switch self {
    case .mention, .message:
      Theme.shared.tintColor.opacity(0.80)
    case .reblog:
      Color.teal.opacity(0.80)
    case .follow:
      Color.cyan.opacity(0.80)
    case .favourite:
      Color.red.opacity(0.80)
    case .moderation:
      Color.orange.opacity(0.80)
    }
  }

  func menuTitle() -> LocalizedStringKey {
    switch self {
    case .mention:
      "notifications.menu-title.mention"
    case .reblog:
      "notifications.menu-title.reblog"
    case .follow:
      "notifications.menu-title.follow"
    case .favourite:
      "notifications.menu-title.favorite"
    case .message:
      "warpnet.notifications.menu-title.message"
    case .moderation:
      "warpnet.notifications.menu-title.moderation"
    }
  }
}
