import AppAccount
import BackgroundTasks
import DesignSystem
import Env
import Foundation
import Models
import Network
import UIKit
import UserNotifications

@MainActor
enum BackgroundRefresh {
  static let identifier = "site.warpnet.warpfone.refresh"
  private static let interval: TimeInterval = 15 * 60

  static func register() {
    BGTaskScheduler.shared.register(forTaskWithIdentifier: identifier, using: .main) { task in
      let pending = PendingTask(value: task)
      Task { @MainActor in
        run(pending)
      }
    }
  }

  static func schedule() {
    let request = BGAppRefreshTaskRequest(identifier: identifier)
    request.earliestBeginDate = Date(timeIntervalSinceNow: interval)
    try? BGTaskScheduler.shared.submit(request)
  }

  static func requestAuthorization() {
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in }
  }

  private static func run(_ pending: PendingTask) {
    schedule()
    let work = Task { @MainActor in
      await refresh()
    }
    pending.value.expirationHandler = {
      work.cancel()
    }
    Task { @MainActor in
      pending.value.setTaskCompleted(success: await work.value)
    }
  }

  private static func refresh() async -> Bool {
    let pairing = PairingModel.shared
    let accounts = AppAccountsManager.shared
    if accounts.currentClient.isAuth {
      await pairing.resume()
    } else {
      await pairing.restoreIfNeeded()
    }
    let client = accounts.currentClient
    guard client.isAuth else { return false }
    let notifications = try? await client.pushes()
    await SharedPostPublisher.publish(with: client)
    if UIApplication.shared.applicationState == .background {
      await pairing.pause()
    }
    guard let notifications else { return false }
    await deliver(notifications)
    return true
  }

  private static func deliver(_ notifications: [Models.Notification]) async {
    guard let newest = notifications.map(\.id).max() else { return }
    let defaults = UserDefaults.standard
    let lastSeen = defaults.string(forKey: StreamWatcher.lastSeenPushKey)
    defaults.set(max(lastSeen ?? newest, newest), forKey: StreamWatcher.lastSeenPushKey)
    guard let lastSeen else { return }
    let center = UNUserNotificationCenter.current()
    for notification in notifications.filter({ $0.id > lastSeen }).sorted(by: { $0.id < $1.id }) {
      guard let body = body(of: notification) else { continue }
      let content = UNMutableNotificationContent()
      content.title = notification.account.safeDisplayName
      content.body = body
      content.sound = .default
      try? await center.add(UNNotificationRequest(identifier: notification.id, content: content, trigger: nil))
    }
  }

  private static func body(of notification: Models.Notification) -> String? {
    switch notification.supportedType {
    case .favourite:
      String(localized: "warpnet.push.reaction")
    case .reblog:
      String(localized: "warpnet.push.retweet")
    case .follow:
      String(localized: "warpnet.push.follow")
    case .mention:
      notification.status?.content.asRawText.escape() ?? String(localized: "warpnet.push.reply")
    default:
      nil
    }
  }
}

private struct PendingTask: @unchecked Sendable {
  let value: BGTask
}
