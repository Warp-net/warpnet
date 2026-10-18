// Modified by Warpnet. See NOTICE.

import Combine
import DesignSystem
import Env
import Models
import Network
import Observation
import SwiftUI

@MainActor
@Observable public class StatusRowViewModel {
  let status: Status
  let showActions: Bool
  let textDisabled: Bool
  let finalStatus: AnyStatus

  let client: Client
  let routerPath: RouterPath

  private let theme = Theme.shared
  private let userMentionned: Bool

  var isPinned: Bool
  var embeddedStatus: Status?
  var isEmbedLoading: Bool = false
  var isFiltered: Bool = false

  var showDeleteAlert: Bool = false
  var showRedraftAlert: Bool = false

  private(set) var actionsAccountsFetched: Bool = false
  var favoriters: [Account] = []
  var rebloggers: [Account] = []

  private var scrollToId = nil as Binding<String?>?

  // The relationship our user has to the author of this post, if available
  var authorRelationship: Relationship? {
    didSet {
      // if we are newly blocking or muting the author, force collapse post so it goes away
      if let relationship = authorRelationship,
         relationship.blocking || relationship.muting
      {
        lineLimit = 0
      }
    }
  }

  // used by the button to expand a collapsed post
  var isCollapsed: Bool = true {
    didSet {
      recalcCollapse()
    }
  }

  // number of lines to show, nil means show the whole post
  var lineLimit: Int?
  // post length determining if the post should be collapsed
  @ObservationIgnored
  let collapseThresholdLength: Int = 750
  // number of text lines to show on a collpased post
  @ObservationIgnored
  let collapsedLines: Int = 8
  // user preference, set in init
  @ObservationIgnored
  var collapseLongPosts: Bool = false

  private func recalcCollapse() {
    let showCollapseButton = collapseLongPosts && isCollapsed
      && finalStatus.content.asRawText.unicodeScalars.count > collapseThresholdLength
    let newlineLimit = showCollapseButton && isCollapsed ? collapsedLines : nil
    if newlineLimit != lineLimit {
      lineLimit = newlineLimit
    }
  }

  var filter: Filtered? {
    finalStatus.filtered?.first
  }

  var isThread: Bool {
    status.reblog?.inReplyToId != nil || status.reblog?.inReplyToAccountId != nil ||
      status.inReplyToId != nil || status.inReplyToAccountId != nil
  }

  var highlightRowColor: Color {
    if userMentionned {
      theme.secondaryBackgroundColor
    } else {
      theme.primaryBackgroundColor
    }
  }

  public init(status: Status,
              client: Client,
              routerPath: RouterPath,
              showActions: Bool = true,
              textDisabled: Bool = false,
              scrollToId: Binding<String?>? = nil)
  {
    self.status = status
    finalStatus = status.reblog ?? status
    self.client = client
    self.routerPath = routerPath
    self.showActions = showActions
    self.textDisabled = textDisabled
    self.scrollToId = scrollToId
    if let reblog = status.reblog {
      isPinned = reblog.pinned == true
    } else {
      isPinned = status.pinned == true
    }
    if status.mentions.first(where: { $0.id == CurrentAccount.shared.account?.id }) != nil {
      userMentionned = true
    } else {
      userMentionned = false
    }

    isFiltered = filter != nil

    if let url = embededStatusURL(),
       let embed = StatusEmbedCache.shared.get(url: url)
    {
      isEmbedLoading = false
      embeddedStatus = embed
    }

    collapseLongPosts = UserPreferences.shared.collapseLongPosts
    recalcCollapse()
  }

  func navigateToDetail() {
    routerPath.navigate(to: .statusDetailWithStatus(status: status.reblogAsAsStatus ?? status))
  }

  func navigateToAccountDetail(account: Account) {
    routerPath.navigate(to: .accountDetailWithAccount(account: account))
  }

  func goToParent() {
    guard let id = status.inReplyToId else { return }
    if let _ = scrollToId {
      scrollToId?.wrappedValue = id
    } else {
      routerPath.navigate(to: .statusDetail(id: id))
    }
  }

  func loadAuthorRelationship() async {
    let relationships: [Relationship]? = try? await client.get(endpoint: Accounts.relationships(ids: [status.reblog?.account.id ?? status.account.id]))
    authorRelationship = relationships?.first
  }

  private func embededStatusURL() -> URL? {
    let content = finalStatus.content
    if client.isAuth,
       let url = content.statusesURLs.first,
       !StatusEmbedCache.shared.badStatusesURLs.contains(url)
    {
      return url
    }
    return nil
  }

  func loadEmbeddedStatus() async {
    guard embeddedStatus == nil,
          let url = embededStatusURL()
    else {
      if isEmbedLoading {
        isEmbedLoading = false
      }
      return
    }

    if let embed = StatusEmbedCache.shared.get(url: url) {
      isEmbedLoading = false
      embeddedStatus = embed
      return
    }

    do {
      isEmbedLoading = true
      let results: SearchResults = try await client.get(endpoint: Search.search(query: url.absoluteString,
                                                                                type: "statuses"))
      let embed = results.statuses.first
      if let embed {
        StatusEmbedCache.shared.set(url: url, status: embed)
      } else {
        StatusEmbedCache.shared.badStatusesURLs.insert(url)
      }
      withAnimation {
        embeddedStatus = embed
        isEmbedLoading = false
      }
    } catch {
      isEmbedLoading = false
      StatusEmbedCache.shared.badStatusesURLs.insert(url)
    }
  }

  func pin() async {
    guard client.isAuth else { return }
    isPinned = true
    do {
      let status: Status = try await client.post(endpoint: Statuses.pin(id: finalStatus.id))
      updateFromStatus(status: status)
    } catch {
      isPinned = false
    }
  }

  func unPin() async {
    guard client.isAuth else { return }
    isPinned = false
    do {
      let status: Status = try await client.post(endpoint: Statuses.unpin(id: finalStatus.id))
      updateFromStatus(status: status)
    } catch {
      isPinned = true
    }
  }

  func delete() async {
    do {
      StreamWatcher.shared.emmitDeleteEvent(for: status.id)
      _ = try await client.delete(endpoint: Statuses.status(id: status.id))
    } catch {}
  }

  /// Deletes the tweet and opens the editor on its text, media and parent, as warpdroid does.
  func redraft() async {
    let source = status.reblogAsAsStatus ?? status
    do {
      _ = try await client.delete(endpoint: Statuses.status(id: source.id))
      StreamWatcher.shared.emmitDeleteEvent(for: status.id)
      routerPath.presentedSheet = .redraftStatusEditor(status: source)
    } catch {}
  }

  func fetchActionsAccounts() async {
    guard !actionsAccountsFetched else { return }
    do {
      withAnimation(.smooth) {
        actionsAccountsFetched = true
      }
      let favoriters: [Account] = try await client.get(endpoint: Statuses.favoritedBy(id: status.id, maxId: nil))
      let rebloggers: [Account] = try await client.get(endpoint: Statuses.rebloggedBy(id: status.id, maxId: nil))
      withAnimation(.smooth) {
        self.favoriters = favoriters
        self.rebloggers = rebloggers
      }
    } catch {}
  }

  private func updateFromStatus(status: Status) {
    if let reblog = status.reblog {
      isPinned = reblog.pinned == true
    } else {
      isPinned = status.pinned == true
    }
  }
}
