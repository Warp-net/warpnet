// Modified by Warpnet. See NOTICE.

import Foundation
import Models

public enum Statuses: Endpoint {
  case postStatus(json: StatusData)
  case editStatus(id: String, json: StatusData)
  case status(id: String)
  case context(id: String)
  case favorite(id: String)
  case react(id: String, emoji: String)
  case unfavorite(id: String)
  case reblog(id: String)
  case unreblog(id: String)
  case rebloggedBy(id: String, maxId: String?)
  case favoritedBy(id: String, maxId: String?)
  case pin(id: String)
  case unpin(id: String)
  case bookmark(id: String)
  case unbookmark(id: String)
  case report(accountId: String, statusId: String?, comment: String)

  public func path() -> String {
    switch self {
    case .postStatus:
      "statuses"
    case let .status(id):
      "statuses/\(id)"
    case let .editStatus(id, _):
      "statuses/\(id)"
    case let .context(id):
      "statuses/\(id)/context"
    case let .favorite(id):
      "statuses/\(id)/favourite"
    case let .react(id, emoji):
      "statuses/\(id)/emoji_reactions/\(emoji)"
    case let .unfavorite(id):
      "statuses/\(id)/unfavourite"
    case let .reblog(id):
      "statuses/\(id)/reblog"
    case let .unreblog(id):
      "statuses/\(id)/unreblog"
    case let .rebloggedBy(id, _):
      "statuses/\(id)/reblogged_by"
    case let .favoritedBy(id, _):
      "statuses/\(id)/favourited_by"
    case let .pin(id):
      "statuses/\(id)/pin"
    case let .unpin(id):
      "statuses/\(id)/unpin"
    case let .bookmark(id):
      "statuses/\(id)/bookmark"
    case let .unbookmark(id):
      "statuses/\(id)/unbookmark"
    case .report:
      "reports"
    }
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .rebloggedBy(_, maxId):
      return makePaginationParam(sinceId: nil, maxId: maxId, mindId: nil)
    case let .favoritedBy(_, maxId):
      return makePaginationParam(sinceId: nil, maxId: maxId, mindId: nil)
    case let .report(accountId, statusId, comment):
      var items: [URLQueryItem] = [.init(name: "account_id", value: accountId)]
      if let statusId {
        items.append(.init(name: "status_ids[]", value: statusId))
      }
      items.append(.init(name: "comment", value: comment))
      return items
    default:
      return nil
    }
  }

  public var jsonValue: Encodable? {
    switch self {
    case let .postStatus(json):
      json
    case let .editStatus(_, json):
      json
    default:
      nil
    }
  }
}

public struct StatusData: Encodable, Sendable {
  public let status: String
  public let inReplyToId: String?
  public let mediaIds: [String]?
  public let poll: PollData?
  public let quotedStatusId: String?

  public struct PollData: Encodable, Sendable {
    public let options: [String]
    public let expires_in: Int

    public init(options: [String], expires_in: Int) {
      self.options = options
      self.expires_in = expires_in
    }
  }

  public init(status: String,
              inReplyToId: String? = nil,
              mediaIds: [String]? = nil,
              poll: PollData? = nil,
              quotedStatusId: String? = nil)
  {
    self.status = status
    self.inReplyToId = inReplyToId
    self.mediaIds = mediaIds
    self.poll = poll
    self.quotedStatusId = quotedStatusId
  }
}
