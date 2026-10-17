// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Network
import Observation
import OSLog

@MainActor
@Observable public class StreamWatcher {
  private var client: Client?
  private var pollTask: Task<Void, Never>?
  private var lastSeenId: String?
  private var hasBaseline = false

  private let pollInterval: Swift.Duration = .seconds(20)

  public enum Stream: String {
    case federated = "public"
    case local
    case user
    case direct
  }

  public var events: [any StreamEvent] = []
  public var unreadNotificationsCount: Int = 0
  public var latestEvent: (any StreamEvent)?

  private let logger = Logger(subsystem: "site.warpnet.warpfone", category: "stream")

  public static let shared = StreamWatcher()
  public static let lastSeenPushKey = "warpnet.pushes.last-seen"

  private init() {}

  public func setClient(client: Client, instanceStreamingURL _: URL?) {
    stopWatching()
    self.client = client
    lastSeenId = nil
    hasBaseline = false
  }

  public func watch(streams _: [Stream]) {
    guard client?.isAuth == true, pollTask == nil else { return }
    pollTask = Task { [weak self] in
      while !Task.isCancelled {
        await self?.poll()
        try? await Task.sleep(for: self?.pollInterval ?? .seconds(20))
      }
    }
  }

  public func stopWatching() {
    pollTask?.cancel()
    pollTask = nil
  }

  private func poll() async {
    guard let client, client.isAuth else { return }
    let notifications: [Models.Notification]
    do {
      notifications = try await client.pushes()
    } catch {
      logger.error("Polling pushes: \(error.localizedDescription, privacy: .public)")
      return
    }
    let newest = notifications.map(\.id).max()
    if let newest {
      UserDefaults.standard.set(max(UserDefaults.standard.string(forKey: Self.lastSeenPushKey) ?? newest, newest),
                                forKey: Self.lastSeenPushKey)
    }
    // The first poll marks what was already there, even when that is nothing.
    guard hasBaseline else {
      hasBaseline = true
      lastSeenId = newest
      return
    }
    let previous = lastSeenId
    let fresh = notifications.filter { notification in previous.map { notification.id > $0 } ?? true }
      .sorted { $0.id < $1.id }
    if let newest {
      lastSeenId = max(previous ?? newest, newest)
    }
    for notification in fresh {
      let event = StreamEventNotification(notification: notification)
      events.append(event)
      latestEvent = event
      unreadNotificationsCount += 1
    }
  }

  public func emmitDeleteEvent(for status: String) {
    let event = StreamEventDelete(status: status)
    self.events.append(event)
    self.latestEvent = event
  }

  public func emmitEditEvent(for status: Status) {
    let event = StreamEventStatusUpdate(status: status)
    self.events.append(event)
    self.latestEvent = event
  }

  public func emmitPostEvent(for status: Status) {
    let event = StreamEventUpdate(status: status)
    self.events.append(event)
    self.latestEvent = event
  }
}
