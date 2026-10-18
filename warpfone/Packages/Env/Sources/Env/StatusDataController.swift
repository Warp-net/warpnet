// Modified by Warpnet. See NOTICE.

import Foundation
import Models
import Network
import Observation
import SwiftUI

@MainActor
public protocol StatusDataControlling {
  var isReblogged: Bool { get set }
  var isBookmarked: Bool { get set }
  var isFavorited: Bool { get set }
  var myReaction: String? { get set }

  var favoritesCount: Int { get set }
  var reblogsCount: Int { get set }
  var repliesCount: Int { get set }

  func toggleBookmark() async
  func toggleReblog() async
  func toggleFavorite() async
}

@MainActor
public final class StatusDataControllerProvider {
  public static let shared = StatusDataControllerProvider()

  private var cache: NSMutableDictionary = [:]

  private struct CacheKey: Hashable {
    let statusId: String
    let client: Client
  }

  public func dataController(for status: any AnyStatus, client: Client) -> StatusDataController {
    let key = CacheKey(statusId: status.id, client: client)
    if let controller = cache[key] as? StatusDataController {
      return controller
    }
    let controller = StatusDataController(status: status, client: client)
    cache[key] = controller
    return controller
  }

  public func updateDataControllers(for statuses: [Status], client: Client) {
    for status in statuses {
      let realStatus: AnyStatus = status.reblog ?? status
      let controller = dataController(for: realStatus, client: client)
      controller.updateFrom(status: realStatus)
    }
  }
}

@MainActor
@Observable public final class StatusDataController: StatusDataControlling {
  private let status: AnyStatus
  private let client: Client

  public var isReblogged: Bool
  public var isBookmarked: Bool
  public var isFavorited: Bool
  public var myReaction: String?
  public var reactions: [String: Int]
  public var content: HTMLString

  public var favoritesCount: Int
  public var reblogsCount: Int
  public var repliesCount: Int

  init(status: AnyStatus, client: Client) {
    self.status = status
    self.client = client

    isReblogged = status.reblogged == true
    isBookmarked = status.bookmarked == true
    isFavorited = status.favourited == true
    myReaction = status.myReaction
    reactions = status.reactions ?? [:]

    reblogsCount = status.reblogsCount
    repliesCount = status.repliesCount
    favoritesCount = status.favouritesCount
    content = status.content
  }

  public func updateFrom(status: AnyStatus) {
    isReblogged = status.reblogged == true
    isBookmarked = status.bookmarked == true
    isFavorited = status.favourited == true
    myReaction = status.myReaction
    reactions = status.reactions ?? [:]

    reblogsCount = status.reblogsCount
    repliesCount = status.repliesCount
    favoritesCount = status.favouritesCount
    content = status.content
  }

  /// A tap takes back the reaction you hold, or reacts with the default heart.
  public func toggleFavorite() async {
    await react(emoji: isFavorited ? myReaction ?? WarpnetReactions.defaultReaction : WarpnetReactions.defaultReaction)
  }

  /// Reacts with `emoji`; reacting with the one you already hold takes it back.
  public func react(emoji: String) async {
    guard client.isAuth else { return }
    let previous = (isFavorited, myReaction, favoritesCount, reactions)
    let isRemoving = isFavorited && myReaction == emoji
    let id = status.id
    withAnimation(.default) {
      if isFavorited, let held = myReaction {
        let count = (reactions[held] ?? 0) - 1
        reactions[held] = count > 0 ? count : nil
      }
      if !isRemoving {
        reactions[emoji, default: 0] += 1
      }
      if !isFavorited {
        favoritesCount += 1
      } else if isRemoving {
        favoritesCount -= 1
      }
      isFavorited = !isRemoving
      myReaction = isRemoving ? nil : emoji
    }
    do {
      let endpoint = isRemoving ? Statuses.unfavorite(id: id) : Statuses.react(id: id, emoji: emoji)
      let status: Status = try await client.post(endpoint: endpoint)
      updateFrom(status: status)
    } catch {
      (isFavorited, myReaction, favoritesCount, reactions) = previous
    }
  }

  public func toggleReblog() async {
    guard client.isAuth else { return }
    isReblogged.toggle()
    let id = status.id
    let endpoint = isReblogged ? Statuses.reblog(id: id) : Statuses.unreblog(id: id)
    withAnimation(.default) {
      reblogsCount += isReblogged ? 1 : -1
    }
    do {
      let status: Status = try await client.post(endpoint: endpoint)
      updateFrom(status: status.reblog ?? status)
    } catch {
      isReblogged.toggle()
      reblogsCount += isReblogged ? -1 : 1
    }
  }

  public func toggleBookmark() async {
    guard client.isAuth else { return }
    isBookmarked.toggle()
    let id = status.id
    let endpoint = isBookmarked ? Statuses.bookmark(id: id) : Statuses.unbookmark(id: id)
    do {
      let status: Status = try await client.post(endpoint: endpoint)
      updateFrom(status: status)
    } catch {
      isBookmarked.toggle()
    }
  }
}
