// Modified by Warpnet. See NOTICE.

import Foundation
import Models
import Network
import SwiftUI

public enum TimelineFilter: Hashable, Equatable, Identifiable {

  case home
  case hashtag(tag: String, accountId: String?)
  case latest
  case resume

  public var id: String {
    title
  }

  public func hash(into hasher: inout Hasher) {
    hasher.combine(id)
  }

  public var supportNewestPagination: Bool {
    true
  }

  public var title: String {
    switch self {
    case .latest:
      "Latest"
    case .resume:
      "Resume"
    case .home:
      "Home"
    case let .hashtag(tag, _):
      "#\(tag)"
    }
  }

  public func localizedTitle() -> LocalizedStringKey {
    switch self {
    case .latest:
      "timeline.latest"
    case .resume:
      "timeline.resume"
    case .home:
      "timeline.home"
    case let .hashtag(tag, _):
      "#\(tag)"
    }
  }

  public func iconName() -> String {
    switch self {
    case .latest:
      "arrow.counterclockwise"
    case .resume:
      "clock.arrow.2.circlepath"
    case .home:
      "house"
    case .hashtag:
      "number"
    }
  }

  public func endpoint(sinceId: String?, maxId: String?, minId: String?, offset _: Int?) -> Endpoint {
    switch self {
    case .latest: return Timelines.home(sinceId: nil, maxId: nil, minId: nil)
    case .resume: return Timelines.home(sinceId: nil, maxId: nil, minId: nil)
    case .home: return Timelines.home(sinceId: sinceId, maxId: maxId, minId: minId)
    case let .hashtag(tag, accountId):
      if let accountId {
        return Accounts.statuses(id: accountId,
                                 sinceId: nil,
                                 tag: tag,
                                 onlyMedia: false,
                                 excludeReplies: false,
                                 excludeReblogs: false,
                                 pinned: nil)
      } else {
        return Timelines.hashtag(tag: tag, additional: nil, maxId: maxId, minId: minId)
      }
    }
  }
}

extension TimelineFilter: Codable {
  enum CodingKeys: String, CodingKey {
    case home
    case hashtag
    case latest
    case resume
  }

  public init(from decoder: Decoder) throws {
    let container = try decoder.container(keyedBy: CodingKeys.self)
    let key = container.allKeys.first
    switch key {
    case .home:
      self = .home
    case .hashtag:
      var nestedContainer = try container.nestedUnkeyedContainer(forKey: .hashtag)
      let tag = try nestedContainer.decode(String.self)
      let accountId = try nestedContainer.decode(String?.self)
      self = .hashtag(
        tag: tag,
        accountId: accountId
      )
    case .latest:
      self = .latest
    default:
      throw DecodingError.dataCorrupted(
        DecodingError.Context(
          codingPath: container.codingPath,
          debugDescription: "Unabled to decode enum."
        )
      )
    }
  }

  public func encode(to encoder: Encoder) throws {
    var container = encoder.container(keyedBy: CodingKeys.self)
    switch self {
    case .home:
      try container.encode(CodingKeys.home.rawValue, forKey: .home)
    case let .hashtag(tag, accountId):
      var nestedContainer = container.nestedUnkeyedContainer(forKey: .hashtag)
      try nestedContainer.encode(tag)
      try nestedContainer.encode(accountId)
    case .latest:
      try container.encode(CodingKeys.latest.rawValue, forKey: .latest)
    case .resume:
      try container.encode(CodingKeys.resume.rawValue, forKey: .latest)
    }
  }
}

extension TimelineFilter: RawRepresentable {
  public init?(rawValue: String) {
    guard let data = rawValue.data(using: .utf8),
          let result = try? JSONDecoder().decode(TimelineFilter.self, from: data)
    else {
      return nil
    }
    self = result
  }

  public var rawValue: String {
    guard let data = try? JSONEncoder().encode(self),
          let result = String(data: data, encoding: .utf8)
    else {
      return "[]"
    }
    return result
  }
}
