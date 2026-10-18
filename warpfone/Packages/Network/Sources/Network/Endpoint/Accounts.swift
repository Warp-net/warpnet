import Foundation
import Models

public enum Accounts: Endpoint {
  case accounts(id: String)
  case favorites(sinceId: String?)
  case bookmarks(sinceId: String?)
  case verifyCredentials
  case updateCredentialsMedia
  case updateCredentials(json: UpdateCredentialsData)
  case statuses(id: String,
                sinceId: String?,
                onlyMedia: Bool,
                excludeReplies: Bool,
                excludeReblogs: Bool,
                pinned: Bool?)
  case relationships(ids: [String])
  case follow(id: String)
  case unfollow(id: String)
  case suggestions
  case followers(id: String, maxId: String?)
  case following(id: String, maxId: String?)
  case block(id: String)
  case unblock(id: String)
  case mute(id: String)
  case unmute(id: String)

  public func path() -> String {
    switch self {
    case let .accounts(id):
      "accounts/\(id)"
    case .favorites:
      "favourites"
    case .bookmarks:
      "bookmarks"
    case .verifyCredentials:
      "accounts/verify_credentials"
    case .updateCredentials, .updateCredentialsMedia:
      "accounts/update_credentials"
    case let .statuses(id, _, _, _, _, _):
      "accounts/\(id)/statuses"
    case .relationships:
      "accounts/relationships"
    case let .follow(id):
      "accounts/\(id)/follow"
    case let .unfollow(id):
      "accounts/\(id)/unfollow"
    case .suggestions:
      "suggestions"
    case let .following(id, _):
      "accounts/\(id)/following"
    case let .followers(id, _):
      "accounts/\(id)/followers"
    case let .block(id):
      "accounts/\(id)/block"
    case let .unblock(id):
      "accounts/\(id)/unblock"
    case let .mute(id):
      "accounts/\(id)/mute"
    case let .unmute(id):
      "accounts/\(id)/unmute"
    }
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .statuses(_, sinceId, onlyMedia, excludeReplies, excludeReblogs, pinned):
      var params: [URLQueryItem] = []
      if let sinceId {
        params.append(.init(name: "max_id", value: sinceId))
      }
      
      params.append(.init(name: "only_media", value: onlyMedia ? "true" : "false"))
      params.append(.init(name: "exclude_replies", value: excludeReplies ? "true" : "false"))
      params.append(.init(name: "exclude_reblogs", value: excludeReblogs ? "true" : "false"))
      
      if let pinned {
        params.append(.init(name: "pinned", value: pinned ? "true" : "false"))
      }
      return params
    case let .relationships(ids):
      return ids.map {
        URLQueryItem(name: "id[]", value: $0)
      }
    case let .followers(_, maxId):
      return makePaginationParam(sinceId: nil, maxId: maxId, mindId: nil)
    case let .following(_, maxId):
      return makePaginationParam(sinceId: nil, maxId: maxId, mindId: nil)
    case let .favorites(sinceId):
      guard let sinceId else { return nil }
      return [.init(name: "max_id", value: sinceId)]
    case let .bookmarks(sinceId):
      guard let sinceId else { return nil }
      return [.init(name: "max_id", value: sinceId)]
    default:
      return nil
    }
  }

  public var jsonValue: Encodable? {
    switch self {
    case let .updateCredentials(json):
      json
    default:
      nil
    }
  }
}

public struct UpdateCredentialsData: Encodable, Sendable {
  public let displayName: String
  public let note: String
  public let website: String

  public init(displayName: String, note: String, website: String) {
    self.displayName = displayName
    self.note = note
    self.website = website
  }
}
