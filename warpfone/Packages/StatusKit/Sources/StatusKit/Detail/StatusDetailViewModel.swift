// Modified by Warpnet. See NOTICE.

import Env
import Foundation
import Models
import Network
import SwiftUI

@MainActor
@Observable class StatusDetailViewModel {
  public var statusId: String?

  var client: Client?
  var routerPath: RouterPath?
  @ObservationIgnored private var didRecordView = false

  enum State {
    case loading, display(statuses: [Status]), error(error: Error)
  }

  var state: State = .loading
  var title: LocalizedStringKey = ""
  var scrollToId: String?

  @ObservationIgnored
  var indentationLevelPreviousCache: [String: UInt] = [:]

  init(statusId: String) {
    state = .loading
    self.statusId = statusId
  }

  init(status: Status) {
    state = .display(statuses: [status])
    title = "status.post-from-\(status.account.displayNameWithoutEmojis)"
    statusId = status.id
    if status.inReplyToId != nil {
      indentationLevelPreviousCache[status.id] = 1
    }
  }

  func fetch() async -> Bool {
    if statusId != nil {
      await fetchStatusDetail(animate: false)
      recordView()
      return true
    }
    return false
  }

  private func recordView() {
    guard let client, let statusId, !didRecordView else { return }
    didRecordView = true
    Task {
      _ = try? await client.post(endpoint: TweetViews.record(statusId: statusId))
    }
  }

  struct ContextData {
    let status: Status
    let context: StatusContext
  }

  private func fetchStatusDetail(animate: Bool) async {
    guard let client, let statusId else { return }
    do {
      let data = try await fetchContextData(client: client, statusId: statusId)
      title = "status.post-from-\(data.status.account.displayNameWithoutEmojis)"
      var statuses = data.context.ancestors
      statuses.append(data.status)
      statuses.append(contentsOf: data.context.descendants)
      cacheReplyTopPrevious(statuses: statuses)
      StatusDataControllerProvider.shared.updateDataControllers(for: statuses, client: client)

      if animate {
        withAnimation {
          state = .display(statuses: statuses)
        }
      } else {
        state = .display(statuses: statuses)
        scrollToId = data.status.id + (data.status.editedAt?.asDate.description ?? "")
      }
    } catch {
      if (error as? ServerError)?.httpCode == 404 || error.isMissingTweet {
        _ = routerPath?.path.popLast()
      } else {
        state = .error(error: error)
      }
    }
  }

  private func fetchContextData(client: Client, statusId: String) async throws -> ContextData {
    async let status: Status = client.get(endpoint: Statuses.status(id: statusId))
    async let context: StatusContext = client.get(endpoint: Statuses.context(id: statusId))
    return try await .init(status: status, context: context)
  }

  private func cacheReplyTopPrevious(statuses: [Status]) {
    indentationLevelPreviousCache = [:]
    for status in statuses {
      if let inReplyToId = status.inReplyToId,
         let prevIndent = indentationLevelPreviousCache[inReplyToId]
      {
        indentationLevelPreviousCache[status.id] = prevIndent + 1
      } else {
        indentationLevelPreviousCache[status.id] = 0
      }
    }
  }

  func handleEvent(event: any StreamEvent, currentAccount: Account?) {
    Task {
      if let event = event as? StreamEventUpdate,
         event.status.account.id == currentAccount?.id
      {
        await fetchStatusDetail(animate: true)
      } else if let event = event as? StreamEventStatusUpdate,
                event.status.account.id == currentAccount?.id
      {
        await fetchStatusDetail(animate: true)
      } else if event is StreamEventDelete {
        await fetchStatusDetail(animate: true)
      }
    }
  }

  func getIndentationLevel(id: String, maxIndent: UInt) -> UInt {
    min(indentationLevelPreviousCache[id] ?? 0, maxIndent)
  }
}
